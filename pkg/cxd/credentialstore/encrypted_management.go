package credentialstore

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"reflect"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/sys/unix"
)

// EncryptionInfo contains public metadata only. Fingerprint identifies the
// wrapped key and connection, and stays unchanged across OAuth refreshes.
type EncryptionInfo struct{ Backend, KeyID, Fingerprint string }

func (s *EncryptedStore) Info(ctx context.Context) (EncryptionInfo, error) {
	var info EncryptionInfo
	err := s.base.locked(ctx, s.name, func(dir *os.File) error {
		r, err := readEncryptedRecord(dir, s.name)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(dataAAD(r.Header))
		info = EncryptionInfo{r.Header.Backend, r.Header.KeyID, hex.EncodeToString(sum[:])}
		return nil
	})
	return info, err
}
func validKeyID(id string) bool {
	decoded, err := hex.DecodeString(id)
	return err == nil && len(decoded) == 16
}
func validUnlockParameters(h encryptedHeader) bool {
	switch h.Backend {
	case "passphrase":
		return h.Memory >= 64*1024 && h.Memory <= 128*1024 && h.Time >= 3 && h.Time <= 6 && h.Parallelism >= 1 && h.Parallelism <= 4 && len(h.Salt) == 16
	case "secret-service":
		return h.Memory == 0 && h.Time == 0 && h.Parallelism == 0 && len(h.Salt) == 0
	}
	return false
}
func newPassphraseHeader(name string, pass []byte) (encryptedHeader, []byte, error) {
	key := make([]byte, chacha20poly1305.KeySize)
	if _, err := rand.Read(key); err != nil {
		return encryptedHeader{}, nil, err
	}
	header, err := protectPassphraseKey(name, pass, key)
	if err != nil {
		clear(key)
		return encryptedHeader{}, nil, err
	}
	return header, key, nil
}
func protectPassphraseKey(name string, pass, key []byte) (encryptedHeader, error) {
	if len(pass) < 12 || len(pass) > 4096 {
		return encryptedHeader{}, fmt.Errorf("choose a passphrase of 12..4096 bytes")
	}
	header := encryptedHeader{Schema: 1, Name: name, Backend: "passphrase", Memory: 64 * 1024, Time: 3, Parallelism: 1, Salt: make([]byte, 16)}
	if _, err := rand.Read(header.Salt); err != nil {
		return encryptedHeader{}, err
	}
	derived := argon2.IDKey(pass, header.Salt, header.Time, header.Memory, header.Parallelism, chacha20poly1305.KeySize)
	defer clear(derived)
	return wrapNewKey(header, derived, key)
}
func wrapNewKey(h encryptedHeader, wrapperKey, key []byte) (encryptedHeader, error) {
	var err error
	h.KeyID, err = newProfileVersion()
	if err != nil {
		return h, err
	}
	h.WrapNonce = make([]byte, chacha20poly1305.NonceSizeX)
	if _, err = rand.Read(h.WrapNonce); err != nil {
		return h, err
	}
	box, err := chacha20poly1305.NewX(wrapperKey)
	if err != nil {
		return h, err
	}
	h.WrappedKey = box.Seal(nil, h.WrapNonce, key, wrapAAD(h))
	return h, nil
}

// Keyring is an optional host-only key-wrapping backend. Implementations must
// never fall back to files or another backend on failure.
type Keyring interface {
	Put(context.Context, string, []byte) error
	Get(context.Context, string) ([]byte, error)
	Delete(context.Context, string) error
}

func (s *EncryptedStore) CreateWithKeyring(ctx context.Context, name string, ring Keyring, credentials Credentials) error {
	if name != s.name || ring == nil {
		return fmt.Errorf("invalid keyring profile")
	}
	return s.base.locked(ctx, name, func(dir *os.File) error {
		for _, file := range []string{name + ".vault", name + ".json"} {
			f, err := openProfileFile(dir, file, unix.O_RDONLY)
			if err == nil {
				_ = f.Close()
				return fmt.Errorf("profile already exists")
			}
			if !errors.Is(err, unix.ENOENT) {
				return err
			}
		}
		var err error
		credentials.Version, err = newProfileVersion()
		if err != nil {
			return err
		}
		credentials.ConnectionVersion, err = newProfileVersion()
		if err != nil {
			return err
		}
		key, wrapper := make([]byte, 32), make([]byte, 32)
		defer clear(key)
		defer clear(wrapper)
		if _, err = rand.Read(key); err != nil {
			return err
		}
		if _, err = rand.Read(wrapper); err != nil {
			return err
		}
		h, err := wrapNewKey(encryptedHeader{Schema: 1, Name: name, Backend: "secret-service"}, wrapper, key)
		if err != nil {
			return err
		}
		r, err := sealEncrypted(h, key, credentials)
		if err != nil {
			return err
		}
		// Verify durable keyring storage before committing ciphertext. On uncertain
		// filesystem persistence retain the keyring item so recovery stays possible.
		if err = ring.Put(ctx, h.KeyID, wrapper); err != nil {
			return err
		}
		stored, err := ring.Get(ctx, h.KeyID)
		defer clear(stored)
		if err != nil || !reflect.DeepEqual(stored, wrapper) {
			return fmt.Errorf("keyring read-back verification failed")
		}
		if err = writeEncrypted(ctx, dir, r); err != nil {
			return err
		}
		s.setKey(key, h)
		return nil
	})
}
func (s *EncryptedStore) UnlockWithKeyring(ctx context.Context, ring Keyring) error {
	return s.base.locked(ctx, s.name, func(dir *os.File) error {
		r, err := readEncryptedRecord(dir, s.name)
		if err != nil {
			return err
		}
		if r.Header.Backend != "secret-service" || ring == nil {
			return ErrUnlock
		}
		wrapper, err := ring.Get(ctx, r.Header.KeyID)
		if err != nil {
			return err
		}
		defer clear(wrapper)
		box, err := chacha20poly1305.NewX(wrapper)
		if err != nil {
			return ErrUnlock
		}
		key, err := box.Open(nil, r.Header.WrapNonce, r.Header.WrappedKey, wrapAAD(r.Header))
		if err != nil {
			return ErrUnlock
		}
		defer clear(key)
		if _, err = openEncrypted(r, key); err != nil {
			return ErrUnlock
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		s.setKey(key, r.Header)
		return nil
	})
}

// EncryptPlaintext explicitly migrates an existing profile. If interrupted after
// ciphertext commit, both records are retained and reads fail closed; repeat the
// same operation to verify the ciphertext before removing plaintext.
func (s *EncryptedStore) EncryptPlaintext(ctx context.Context, pass []byte) error {
	return s.base.locked(ctx, s.name, func(dir *os.File) error {
		plain, err := readProfile(dir, s.name)
		if err != nil {
			return err
		}
		record, err := readEncryptedRecord(dir, s.name)
		var key []byte
		if errors.Is(err, ErrNotFound) {
			h, k, e := newPassphraseHeader(s.name, pass)
			if e != nil {
				return e
			}
			key = k
			defer clear(key)
			plain.Version, err = newProfileVersion()
			if err != nil {
				return err
			}
			plain.ConnectionVersion, err = newProfileVersion()
			if err != nil {
				return err
			}
			record, err = sealEncrypted(h, key, plain)
			if err != nil {
				return err
			}
			if err = writeEncrypted(ctx, dir, record); err != nil {
				return err
			}
			record, err = readEncryptedRecord(dir, s.name)
			if err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else {
			if record.Header.Backend != "passphrase" {
				return ErrMigrationPending
			}
			h := record.Header
			derived := argon2.IDKey(pass, h.Salt, h.Time, h.Memory, h.Parallelism, 32)
			defer clear(derived)
			box, _ := chacha20poly1305.NewX(derived)
			key, err = box.Open(nil, h.WrapNonce, h.WrappedKey, wrapAAD(h))
			if err != nil {
				return ErrUnlock
			}
			defer clear(key)
		}
		saved, err := openEncrypted(record, key)
		if err != nil {
			return ErrUnlock
		}
		a, b := plain, saved
		a.Version, b.Version = "", ""
		a.ConnectionVersion, b.ConnectionVersion = "", ""
		if !reflect.DeepEqual(a, b) {
			return ErrConflict
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		if err = unix.Unlinkat(int(dir.Fd()), s.name+".json", 0); err != nil {
			return err
		}
		if err = dir.Sync(); err != nil {
			return err
		}
		s.setKey(key, record.Header)
		return nil
	})
}

// ChangePassphrase requires an unlocked passphrase-backed store. Existing proxy
// processes must be removed by the caller before changing its wrapping key.
func (s *EncryptedStore) ChangePassphrase(ctx context.Context, pass []byte) error {
	return s.base.locked(ctx, s.name, func(dir *os.File) error {
		value, err := s.load(dir)
		if err != nil {
			return err
		}
		s.mu.Lock()
		backend := s.header.Backend
		s.mu.Unlock()
		if backend != "passphrase" {
			return fmt.Errorf("profile does not use a passphrase")
		}
		key, err := s.UnlockedKey()
		if err != nil {
			return err
		}
		defer clear(key)
		h, err := protectPassphraseKey(s.name, pass, key)
		if err != nil {
			return err
		}
		r, err := sealEncrypted(h, key, value)
		if err != nil {
			return err
		}
		if err = writeEncrypted(ctx, dir, r); err != nil {
			return err
		}
		s.setKey(key, h)
		return nil
	})
}
