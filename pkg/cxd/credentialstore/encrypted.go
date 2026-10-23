package credentialstore

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"runtime"
	"sync"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/sys/unix"
)

var (
	ErrLocked           = errors.New("credential profile is encrypted and locked")
	ErrUnlock           = errors.New("could not unlock credential profile")
	ErrMigrationPending = errors.New("encrypted and plaintext records coexist; complete the explicit encryption migration")
)

// ProfileStore is the common CLI profile boundary. Existing FileStore callers
// and embedding Store implementations retain their original contracts.
type ProfileStore interface {
	Store
	Create(context.Context, string, Credentials) error
	Directory() string
}

type encryptedHeader struct {
	Schema      int    `json:"schema"`
	Name        string `json:"name"`
	KeyID       string `json:"key_id"`
	Backend     string `json:"backend"`
	Memory      uint32 `json:"memory_kib"`
	Time        uint32 `json:"iterations"`
	Parallelism uint8  `json:"parallelism"`
	Salt        []byte `json:"salt"`
	WrapNonce   []byte `json:"wrap_nonce"`
	WrappedKey  []byte `json:"wrapped_key"`
}

type encryptedRecord struct {
	Header     encryptedHeader `json:"header"`
	Nonce      []byte          `json:"nonce"`
	Ciphertext []byte          `json:"ciphertext"`
}

// EncryptedStore stores only authenticated ciphertext and a wrapped random
// data key. An unlocked instance is process-local; no plaintext key file or
// additional persistent daemon is created.
type EncryptedStore struct {
	base   *FileStore
	name   string
	mu     sync.Mutex
	key    []byte
	header encryptedHeader
}

func OpenProfile(directory, name string) (ProfileStore, error) {
	s, err := NewFileStore(directory)
	if err != nil {
		return nil, err
	}
	if err := ValidateProfileName(name); err != nil {
		return nil, err
	}
	var encrypted bool
	dir, err := s.openDirectory()
	if err != nil {
		return nil, err
	}
	defer func() { _ = dir.Close() }()
	err = func() error {
		_, err := readEncryptedRecord(dir, name)
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		encrypted = true
		if f, err := openProfileFile(dir, name+".json", unix.O_RDONLY); err == nil {
			_ = f.Close()
			return ErrMigrationPending
		} else if !errors.Is(err, unix.ENOENT) {
			return err
		}
		return nil
	}()
	if err != nil {
		return nil, err
	}
	if encrypted {
		return NewEncryptedStore(directory, name)
	}
	return s, nil
}

func profileVaultPresent(dir *os.File, name string) (bool, error) {
	f, err := openProfileFile(dir, name+".vault", unix.O_RDONLY)
	if errors.Is(err, unix.ENOENT) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, f.Close()
}

func NewEncryptedStore(directory, name string) (*EncryptedStore, error) {
	if err := ValidateProfileName(name); err != nil {
		return nil, err
	}
	base, err := NewFileStore(directory)
	if err != nil {
		return nil, err
	}
	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("encrypted profiles require a verified Linux filesystem")
	}
	dir, err := base.openDirectory()
	if err != nil {
		return nil, err
	}
	defer func() { _ = dir.Close() }()
	var fs unix.Statfs_t
	if err := unix.Fstatfs(int(dir.Fd()), &fs); err != nil {
		return nil, err
	}
	switch int64(fs.Type) {
	case 0xEF53, 0x01021994, 0x9123683E, 0x58465342, 0x794C7630, 0x2fc12fc1:
	default:
		return nil, fmt.Errorf("encrypted profile filesystem is not verified; use private Linux storage")
	}
	return &EncryptedStore{base: base, name: name}, nil
}

func (s *EncryptedStore) Directory() string { return s.base.Directory() }

func readEncryptedRecord(dir *os.File, name string) (encryptedRecord, error) {
	var value encryptedRecord
	f, err := openProfileFile(dir, name+".vault", unix.O_RDONLY)
	if errors.Is(err, unix.ENOENT) {
		return value, ErrNotFound
	}
	if err != nil {
		return value, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, (2<<20)+1))
	if err != nil || len(data) > 2<<20 || json.Unmarshal(data, &value) != nil {
		return value, fmt.Errorf("invalid encrypted profile")
	}
	h := value.Header
	if h.Schema != 1 || h.Name != name || !validKeyID(h.KeyID) || !validUnlockParameters(h) || len(h.WrapNonce) != chacha20poly1305.NonceSizeX || len(h.WrappedKey) != chacha20poly1305.KeySize+chacha20poly1305.Overhead || len(value.Nonce) != chacha20poly1305.NonceSizeX || len(value.Ciphertext) < chacha20poly1305.Overhead {
		return encryptedRecord{}, fmt.Errorf("unsupported encrypted profile or key-derivation parameters")
	}
	return value, nil
}

func wrapAAD(h encryptedHeader) []byte {
	h.WrapNonce, h.WrappedKey = nil, nil
	data, _ := json.Marshal(h)
	return append([]byte("oaiso/profile/key/v1\x00"), data...)
}

func dataAAD(h encryptedHeader) []byte {
	data, _ := json.Marshal(h)
	return append([]byte("oaiso/profile/data/v1\x00"), data...)
}

func (s *EncryptedStore) setKey(key []byte, header encryptedHeader) {
	s.mu.Lock()
	defer s.mu.Unlock()
	clear(s.key)
	s.key = append([]byte{}, key...)
	s.header = header
}

// Close drops the store's key reference and clears its owned byte slice. Go
// cannot guarantee that every temporary memory copy has been zeroed.
func (s *EncryptedStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	clear(s.key)
	s.key = nil
	return nil
}

// UnlockedKey returns an owned data-key copy solely for explicit host-to-proxy
// IPC. Never put its result in argv, environment, metadata, files or logs.
func (s *EncryptedStore) UnlockedKey() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.key) != chacha20poly1305.KeySize {
		return nil, ErrLocked
	}
	return append([]byte{}, s.key...), nil
}

func (s *EncryptedStore) Unlock(ctx context.Context, passphrase []byte) error {
	return s.base.locked(ctx, s.name, func(dir *os.File) error {
		record, err := readEncryptedRecord(dir, s.name)
		if err != nil {
			return err
		}
		h := record.Header
		if h.Backend != "passphrase" || len(passphrase) > 4096 {
			return ErrUnlock
		}
		derived := argon2.IDKey(passphrase, h.Salt, h.Time, h.Memory, h.Parallelism, chacha20poly1305.KeySize)
		defer clear(derived)
		box, _ := chacha20poly1305.NewX(derived)
		key, err := box.Open(nil, h.WrapNonce, h.WrappedKey, wrapAAD(h))
		if err != nil {
			return ErrUnlock
		}
		defer clear(key)
		if _, err := openEncrypted(record, key); err != nil {
			return ErrUnlock
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		s.setKey(key, h)
		return nil
	})
}

func (s *EncryptedStore) UnlockWithKey(ctx context.Context, key []byte) error {
	if len(key) != chacha20poly1305.KeySize {
		return ErrUnlock
	}
	return s.base.locked(ctx, s.name, func(dir *os.File) error {
		record, err := readEncryptedRecord(dir, s.name)
		if err != nil {
			return err
		}
		if _, err := openEncrypted(record, key); err != nil {
			return ErrUnlock
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		s.setKey(key, record.Header)
		return nil
	})
}

func openEncrypted(record encryptedRecord, key []byte) (Credentials, error) {
	box, err := chacha20poly1305.NewX(key)
	if err != nil {
		return Credentials{}, ErrUnlock
	}
	data, err := box.Open(nil, record.Nonce, record.Ciphertext, dataAAD(record.Header))
	if err != nil {
		return Credentials{}, ErrUnlock
	}
	defer clear(data)
	var value fileProfile
	if json.Unmarshal(data, &value) != nil || value.Schema != 1 || value.Name != record.Header.Name {
		return Credentials{}, ErrUnlock
	}
	if err := validateFileCredentials(value.Credentials); err != nil {
		return Credentials{}, ErrUnlock
	}
	return value.Credentials, nil
}

func sealEncrypted(header encryptedHeader, key []byte, credentials Credentials) (encryptedRecord, error) {
	if err := validateFileCredentials(credentials); err != nil {
		return encryptedRecord{}, err
	}
	data, err := json.Marshal(fileProfile{Schema: 1, Name: header.Name, Credentials: credentials})
	if err != nil || len(data) > 1<<20 {
		return encryptedRecord{}, fmt.Errorf("profile exceeds supported size")
	}
	defer clear(data)
	nonce := make([]byte, chacha20poly1305.NonceSizeX)
	if _, err := rand.Read(nonce); err != nil {
		return encryptedRecord{}, err
	}
	box, err := chacha20poly1305.NewX(key)
	if err != nil {
		return encryptedRecord{}, err
	}
	return encryptedRecord{Header: header, Nonce: nonce, Ciphertext: box.Seal(nil, nonce, data, dataAAD(header))}, nil
}

func writeEncrypted(ctx context.Context, dir *os.File, record encryptedRecord) error {
	data, err := json.Marshal(record)
	if err != nil || len(data) > 2<<20 {
		return fmt.Errorf("encrypted profile exceeds supported size")
	}
	return writeProfileData(ctx, dir, record.Header.Name+".vault", data)
}

// CreateWithPassphrase creates only a new encrypted connection; it does not
// replace plaintext or encrypted profiles and never writes plaintext to disk.
func (s *EncryptedStore) CreateWithPassphrase(ctx context.Context, name string, passphrase []byte, credentials Credentials) error {
	if name != s.name || len(passphrase) < 12 || len(passphrase) > 4096 {
		return fmt.Errorf("choose a passphrase of 12..4096 bytes for this profile")
	}
	return s.base.locked(ctx, name, func(dir *os.File) error {
		for _, file := range []string{name + ".vault", name + ".json"} {
			if f, err := openProfileFile(dir, file, unix.O_RDONLY); err == nil {
				_ = f.Close()
				return fmt.Errorf("profile already exists; choose a new profile name")
			} else if !errors.Is(err, unix.ENOENT) {
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
		if err := validateFileCredentials(credentials); err != nil {
			return err
		}
		header, key, err := newPassphraseHeader(name, passphrase)
		if err != nil {
			return err
		}
		defer clear(key)
		record, err := sealEncrypted(header, key, credentials)
		if err != nil {
			return err
		}
		if err := writeEncrypted(ctx, dir, record); err != nil {
			return err
		}
		s.setKey(key, header)
		return nil
	})
}

func (s *EncryptedStore) Create(context.Context, string, Credentials) error {
	return fmt.Errorf("use encrypted profile creation with an explicit unlocking method")
}

func (s *EncryptedStore) load(dir *os.File) (Credentials, error) {
	if f, err := openProfileFile(dir, s.name+".json", unix.O_RDONLY); err == nil {
		_ = f.Close()
		return Credentials{}, ErrMigrationPending
	} else if !errors.Is(err, unix.ENOENT) {
		return Credentials{}, err
	}
	record, err := readEncryptedRecord(dir, s.name)
	if err != nil {
		return Credentials{}, err
	}
	s.mu.Lock()
	key := append([]byte{}, s.key...)
	header := s.header
	s.mu.Unlock()
	defer clear(key)
	if len(key) != chacha20poly1305.KeySize {
		return Credentials{}, ErrLocked
	}
	if sha256.Sum256(dataAAD(header)) != sha256.Sum256(dataAAD(record.Header)) {
		return Credentials{}, ErrLocked
	}
	return openEncrypted(record, key)
}

func (s *EncryptedStore) Load(ctx context.Context, name string) (Credentials, error) {
	if name != s.name {
		return Credentials{}, ErrNotFound
	}
	var value Credentials
	err := s.base.locked(ctx, name, func(dir *os.File) (err error) { value, err = s.load(dir); return err })
	return value, err
}

func (s *EncryptedStore) SaveRefresh(ctx context.Context, name, expected, provider string, next Credentials) (string, error) {
	if name != s.name {
		return "", ErrNotFound
	}
	var version string
	err := s.base.locked(ctx, name, func(dir *os.File) error {
		old, err := s.load(dir)
		if err != nil {
			return err
		}
		if old.Version != expected || old.ConnectionVersion != next.ConnectionVersion {
			return ErrConflict
		}
		switch provider {
		case "openai":
			if old.OpenAIOAuth == nil || next.OpenAIOAuth == nil || old.OpenAIAPIKey != next.OpenAIAPIKey || old.AnthropicAPIKey != next.AnthropicAPIKey || !reflect.DeepEqual(old.AnthropicOAuth, next.AnthropicOAuth) {
				return fmt.Errorf("refresh cannot change another provider or authentication mode")
			}
		case "anthropic":
			if old.AnthropicOAuth == nil || next.AnthropicOAuth == nil || old.AnthropicAPIKey != next.AnthropicAPIKey || old.OpenAIAPIKey != next.OpenAIAPIKey || !reflect.DeepEqual(old.OpenAIOAuth, next.OpenAIOAuth) {
				return fmt.Errorf("refresh cannot change another provider or authentication mode")
			}
		default:
			return fmt.Errorf("unsupported refresh provider")
		}
		version, err = newProfileVersion()
		if err != nil {
			return err
		}
		next.Version = version
		s.mu.Lock()
		key := append([]byte{}, s.key...)
		header := s.header
		s.mu.Unlock()
		defer clear(key)
		record, err := sealEncrypted(header, key, next)
		if err != nil {
			return err
		}
		return writeEncrypted(ctx, dir, record)
	})
	if err != nil {
		return "", err
	}
	return version, nil
}
