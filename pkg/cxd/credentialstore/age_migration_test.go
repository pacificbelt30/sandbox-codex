package credentialstore

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"filippo.io/age"
)

type legacyFixture struct {
	Passphrase  string
	WrappingKey []byte
	Vault       json.RawMessage
}

func installLegacyFixture(t *testing.T, backend string) (*EncryptedStore, legacyFixture, *memoryKeyring) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "legacy-"+backend+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture legacyFixture
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	store, err := NewEncryptedStore(filepath.Join(t.TempDir(), "profile"), "work")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err = os.WriteFile(filepath.Join(store.Directory(), "work.vault"), fixture.Vault, 0600); err != nil {
		t.Fatal(err)
	}
	ring := &memoryKeyring{keys: map[string][]byte{}}
	var record encryptedRecord
	if err = json.Unmarshal(fixture.Vault, &record); err != nil {
		t.Fatal(err)
	}
	ring.keys[record.Header.KeyID] = fixture.WrappingKey
	return store, fixture, ring
}
func TestLegacyProfilesMigrateExplicitlyToAge(t *testing.T) {
	for _, backend := range []string{"passphrase", "keyring"} {
		t.Run(backend, func(t *testing.T) {
			ctx := context.Background()
			s, fixture, ring := installLegacyFixture(t, backend)
			info, err := s.Info(ctx)
			if err != nil || info.Format != "legacy-v1" {
				t.Fatal("legacy format not recognized", err)
			}
			if backend == "passphrase" {
				err = s.Unlock(ctx, []byte(fixture.Passphrase))
			} else {
				err = s.UnlockWithKeyring(ctx, ring)
			}
			if err != nil {
				t.Fatal("old writer output could not be read", err)
			}
			before, err := s.Load(ctx, "work")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.SaveRefresh(ctx, "work", before.Version, "openai", before); !errors.Is(err, ErrMigrationRequired) {
				t.Fatal("legacy writer was not disabled")
			}
			if backend == "passphrase" {
				original, _ := os.ReadFile(filepath.Join(s.Directory(), "work.vault"))
				if err = s.MigrateToAge(ctx, []byte("wrong sufficiently long passphrase"), ring); !errors.Is(err, ErrUnlock) {
					t.Fatal("wrong migration password accepted")
				}
				unchanged, _ := os.ReadFile(filepath.Join(s.Directory(), "work.vault"))
				if !bytes.Equal(original, unchanged) {
					t.Fatal("failed migration changed storage")
				}
			}
			if err = s.MigrateToAge(ctx, []byte(fixture.Passphrase), ring); err != nil {
				t.Fatal(err)
			}
			after, err := s.Load(ctx, "work")
			if err != nil || !sameCredentialContent(before, after) || before.Version == after.Version || before.ConnectionVersion == after.ConnectionVersion {
				t.Fatal("migration lost credentials or retained old generations", err)
			}
			next, err := s.Info(ctx)
			if err != nil || next.Format != "age" || next.Fingerprint == info.Fingerprint {
				t.Fatal("age metadata was not updated", err)
			}
			backup := filepath.Join(s.Directory(), "work.vault.v1-backup")
			backupInfo, err := os.Stat(backup)
			if err != nil || backupInfo.Mode().Perm() != 0600 {
				t.Fatal("private encrypted recovery backup missing")
			}
			data, _ := os.ReadFile(backup)
			var saved encryptedRecord
			if json.Unmarshal(data, &saved) != nil || saved.Header.Schema != 1 || bytes.Contains(data, []byte(before.OpenAIOAuth.AccessToken)) {
				t.Fatal("backup is not original encrypted data")
			}
			_ = s.Close()
			if backend == "passphrase" {
				err = s.Unlock(ctx, []byte(fixture.Passphrase))
			} else {
				err = s.UnlockWithKeyring(ctx, ring)
			}
			if err != nil {
				t.Fatal("new format did not reopen", err)
			}
			if _, err = s.SaveRefresh(ctx, "work", before.Version, "openai", before); !errors.Is(err, ErrConflict) {
				t.Fatal("old generation overwrote migrated credentials")
			}
			if err = s.MigrateToAge(ctx, []byte(fixture.Passphrase), ring); err != nil {
				t.Fatal("migration retry failed", err)
			}
			repeated, _ := s.Load(ctx, "work")
			if !reflect.DeepEqual(repeated, after) {
				t.Fatal("retry changed the new connection")
			}
		})
	}
}
func TestLegacyMigrationKeepsDataOnCancellationAndBackupConflict(t *testing.T) {
	s, fixture, ring := installLegacyFixture(t, "passphrase")
	path := filepath.Join(s.Directory(), "work.vault")
	original, _ := os.ReadFile(path)
	canceled, stop := context.WithCancel(context.Background())
	stop()
	if err := s.MigrateToAge(canceled, []byte(fixture.Passphrase), ring); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled migration accepted")
	}
	if err := os.WriteFile(path+".v1-backup", []byte("unrelated private encrypted backup"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.MigrateToAge(context.Background(), []byte(fixture.Passphrase), ring); err == nil {
		t.Fatal("conflicting backup overwritten")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(original, after) {
		t.Fatal("failed migration overwrote old profile")
	}
}
func TestAgeStorageAuthenticatesPublicKeyReencryption(t *testing.T) {
	s, value := encryptedFixture(t)
	ctx := context.Background()
	dir, err := s.base.openDirectory()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dir.Close() }()
	record, err := readEncryptedRecord(dir, "work")
	if err != nil {
		t.Fatal(err)
	}
	recipient, err := age.ParseX25519Recipient(record.Header.Recipient)
	if err != nil {
		t.Fatal(err)
	}
	value.OpenAIOAuth.AccessToken = "forged-but-valid-age-ciphertext"
	plain, _ := json.Marshal(fileProfile{Schema: 1, Name: "work", Credentials: value})
	defer clear(plain)
	record.Ciphertext, err = ageEncrypt(plain, recipient)
	if err != nil {
		t.Fatal(err)
	}
	// Knowing the public recipient lets a writer make valid age ciphertext, but
	// must not let it create the separate authenticated store record.
	if err = writeEncrypted(ctx, dir, record); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Load(ctx, "work"); !errors.Is(err, ErrUnlock) {
		t.Fatal("public-key-only forgery bypassed store authentication")
	}
}
func TestAgeIdentityWorkFactorIsBounded(t *testing.T) {
	s, _ := encryptedFixture(t)
	path := filepath.Join(s.Directory(), "work.vault")
	data, _ := os.ReadFile(path)
	var record encryptedRecord
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	original := append([]byte{}, record.Header.WrappedKey...)
	record.Header.WrappedKey = bytes.Replace(record.Header.WrappedKey, []byte(" 17\n"), []byte(" 31\n"), 1)
	if bytes.Equal(original, record.Header.WrappedKey) {
		t.Fatal("fixture did not alter the age work factor")
	}
	data, _ = json.Marshal(record)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.Unlock(context.Background(), []byte("correct horse battery staple")); !errors.Is(err, ErrUnlock) {
		t.Fatal("unbounded scrypt header accepted")
	}
}
func TestUnlockKeyStreamRejectsTruncationAndExcessiveLength(t *testing.T) {
	s, _ := encryptedFixture(t)
	key, err := s.UnlockedKey()
	if err != nil {
		t.Fatal(err)
	}
	defer clear(key)
	var output bytes.Buffer
	if err = WriteUnlockKey(&output, key); err != nil {
		t.Fatal(err)
	}
	decoded, err := ReadUnlockKey(bytes.NewReader(output.Bytes()))
	defer clear(decoded)
	if err != nil || !bytes.Equal(decoded, key) {
		t.Fatal("opaque identity stream failed", err)
	}
	var oversized [4]byte
	binary.BigEndian.PutUint32(oversized[:], UnlockKeyMaxSize+1)
	for _, input := range [][]byte{nil, {0, 0}, oversized[:], output.Bytes()[:output.Len()-1]} {
		if _, err = ReadUnlockKey(bytes.NewReader(input)); err == nil {
			t.Fatal("bad key stream accepted")
		}
	}
}
func TestAgeCLIInteroperability(t *testing.T) {
	binaryPath := os.Getenv("OAISO_AGE_CLI")
	if binaryPath == "" {
		t.Skip("set OAISO_AGE_CLI to an independently built official age CLI")
	}
	s, want := encryptedFixture(t)
	key, err := s.UnlockedKey()
	if err != nil {
		t.Fatal(err)
	}
	defer clear(key)
	secret, _, err := parseAgeSecret(key)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(secret.AuthenticationKey)
	dir, err := s.base.openDirectory()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dir.Close() }()
	record, err := readEncryptedRecord(dir, "work")
	if err != nil {
		t.Fatal(err)
	}
	dataPath := filepath.Join(t.TempDir(), "profile.age")
	if err = os.WriteFile(dataPath, record.Ciphertext, 0600); err != nil {
		t.Fatal(err)
	}
	// The private identity is supplied through stdin, never a file or argv.
	command := exec.CommandContext(context.Background(), binaryPath, "--decrypt", "--identity", "-", dataPath)
	command.Stdin = bytes.NewBufferString(secret.Identity + "\n")
	var plaintext bytes.Buffer
	command.Stdout = &plaintext
	defer clear(plaintext.Bytes())
	command.Stderr = io.Discard
	if err = command.Run(); err != nil {
		t.Fatal("official age CLI could not decrypt the profile", err)
	}
	got, err := decodeEncryptedCredentials(plaintext.Bytes(), "work")
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatal("official CLI recovered different credential data")
	}
}
