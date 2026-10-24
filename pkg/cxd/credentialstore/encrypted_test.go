package credentialstore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func encryptedFixture(t *testing.T) (*EncryptedStore, Credentials) {
	t.Helper()
	s, err := NewEncryptedStore(filepath.Join(t.TempDir(), "profiles"), "work")
	if err != nil {
		t.Fatal(err)
	}
	value := Credentials{OpenAIOAuth: &OpenAIOAuth{AccessToken: "fixture-real-access", RefreshToken: "fixture-real-refresh", IDToken: "fixture-real-id"}}
	if err := s.CreateWithPassphrase(context.Background(), "work", []byte("correct horse battery staple"), value); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	value, err = s.Load(context.Background(), "work")
	if err != nil {
		t.Fatal(err)
	}
	return s, value
}

func TestEncryptedStoreNeverWritesPlaintextAndRequiresUnlock(t *testing.T) {
	s, value := encryptedFixture(t)
	data, err := os.ReadFile(filepath.Join(s.Directory(), "work.vault"))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{value.OpenAIOAuth.AccessToken, value.OpenAIOAuth.RefreshToken, value.OpenAIOAuth.IDToken, value.ConnectionVersion} {
		if bytes.Contains(data, []byte(secret)) {
			t.Fatal("encrypted file contains credential plaintext")
		}
	}
	if _, err := os.Stat(filepath.Join(s.Directory(), "work.json")); !os.IsNotExist(err) {
		t.Fatal("encrypted creation left a plaintext profile")
	}
	plain, err := NewFileStore(s.Directory())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plain.Load(context.Background(), "work"); !errors.Is(err, ErrLocked) {
		t.Fatal("plaintext reader bypassed encryption")
	}
	reopened, err := NewEncryptedStore(s.Directory(), "work")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	if _, err := reopened.Load(context.Background(), "work"); !errors.Is(err, ErrLocked) {
		t.Fatal("reopened store did not start locked")
	}
	if err := reopened.Unlock(context.Background(), []byte("incorrect passphrase")); !errors.Is(err, ErrUnlock) {
		t.Fatal("wrong passphrase unlocked the profile")
	}
	if err := reopened.Unlock(context.Background(), []byte("correct horse battery staple")); err != nil {
		t.Fatal(err)
	}
	next, err := reopened.Load(context.Background(), "work")
	if err != nil || next.Version != value.Version || next.ConnectionVersion != value.ConnectionVersion || next.OpenAIOAuth.AccessToken != value.OpenAIOAuth.AccessToken {
		t.Fatal("reopening changed credentials or generations")
	}
	if err := plain.Create(context.Background(), "work", Credentials{OpenAIAPIKey: "unexpected"}); !errors.Is(err, ErrLocked) {
		t.Fatal("plaintext creation shadowed an encrypted profile")
	}
}

func TestEncryptedRefreshCASAndConcurrentWriters(t *testing.T) {
	s, initial := encryptedFixture(t)
	key, err := s.UnlockedKey()
	if err != nil {
		t.Fatal(err)
	}
	defer clear(key)
	other, err := NewEncryptedStore(s.Directory(), "work")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = other.Close() }()
	if err := other.UnlockWithKey(context.Background(), key); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i, store := range []*EncryptedStore{s, other} {
		wg.Add(1)
		go func(index int, store *EncryptedStore) {
			defer wg.Done()
			value := initial
			oauth := *value.OpenAIOAuth
			value.OpenAIOAuth = &oauth
			value.OpenAIOAuth.AccessToken = string(rune('a'+index)) + "-rotated-access"
			_, err := store.SaveRefresh(context.Background(), "work", initial.Version, "openai", value)
			results <- err
		}(i, store)
	}
	wg.Wait()
	close(results)
	passed, conflicted := 0, 0
	for err := range results {
		if err == nil {
			passed++
		} else if errors.Is(err, ErrConflict) {
			conflicted++
		} else {
			t.Fatal(err)
		}
	}
	if passed != 1 || conflicted != 1 {
		t.Fatal("concurrent writers did not preserve encrypted CAS")
	}
	next, err := s.Load(context.Background(), "work")
	if err != nil || next.Version == initial.Version || next.ConnectionVersion != initial.ConnectionVersion {
		t.Fatal("refresh changed connection generation or reused the credential version")
	}
	copy := next
	copy.AnthropicAPIKey = "unexpected-provider"
	if _, err := s.SaveRefresh(context.Background(), "work", next.Version, "openai", copy); err == nil {
		t.Fatal("refresh changed a different provider")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.SaveRefresh(canceled, "work", next.Version, "openai", next); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled encrypted update was published")
	}
}

func TestEncryptedStoreRejectsTamperingAndExcessiveKDF(t *testing.T) {
	s, _ := encryptedFixture(t)
	path := filepath.Join(s.Directory(), "work.vault")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var record encryptedRecord
	if err := json.Unmarshal(original, &record); err != nil {
		t.Fatal(err)
	}
	record.Ciphertext[0] ^= 1
	data, _ := json.Marshal(record)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(context.Background(), "work"); !errors.Is(err, ErrUnlock) {
		t.Fatal("tampered ciphertext was accepted")
	}
	if err := json.Unmarshal(original, &record); err != nil {
		t.Fatal(err)
	}
	record.Header.Memory = ^uint32(0)
	data, _ = json.Marshal(record)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	reopened, _ := NewEncryptedStore(s.Directory(), "work")
	if err := reopened.Unlock(context.Background(), []byte("correct horse battery staple")); err == nil {
		t.Fatal("unbounded KDF parameters were accepted")
	}
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.Directory(), "work.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenProfile(s.Directory(), "work"); !errors.Is(err, ErrMigrationPending) {
		t.Fatal("coexisting plaintext was silently selected")
	}
}

func TestEncryptedMigrationAndPassphraseChange(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "profile")
	plain, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err = plain.Create(ctx, "work", Credentials{OpenAIAPIKey: "fixture-migration-key"}); err != nil {
		t.Fatal(err)
	}
	original, _ := plain.Load(ctx, "work")
	store, err := NewEncryptedStore(dir, "work")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	oldPass, newPass := []byte("old sufficiently long passphrase"), []byte("new sufficiently long passphrase")
	if err = store.EncryptPlaintext(ctx, oldPass); err != nil {
		t.Fatal(err)
	}
	saved, err := store.Load(ctx, "work")
	if err != nil || saved.ConnectionVersion == original.ConnectionVersion || saved.OpenAIAPIKey != original.OpenAIAPIKey {
		t.Fatal("migration failed to rotate connection or preserve credentials", err)
	}
	key, _ := store.UnlockedKey()
	defer clear(key)
	if err = store.ChangePassphrase(ctx, newPass); err != nil {
		t.Fatal(err)
	}
	nextKey, _ := store.UnlockedKey()
	defer clear(nextKey)
	if !bytes.Equal(key, nextKey) {
		t.Fatal("passphrase change unexpectedly rotated data key")
	}
	after, _ := store.Load(ctx, "work")
	if after.ConnectionVersion != saved.ConnectionVersion || after.Version != saved.Version {
		t.Fatal("passphrase change changed authentication generations")
	}
	_ = store.Close()
	if err = store.Unlock(ctx, oldPass); !errors.Is(err, ErrUnlock) {
		t.Fatal("old passphrase still works")
	}
	if err = store.Unlock(ctx, newPass); err != nil {
		t.Fatal(err)
	}
}
func TestInterruptedMigrationRequiresExplicitMatchingRecovery(t *testing.T) {
	ctx := context.Background()
	s, initial := encryptedFixture(t)
	// Simulate a crash after verified ciphertext was committed but before the
	// original plaintext record was removed.
	dir, err := s.base.openDirectory()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dir.Close() }()
	plain := initial
	plain.Version = "pre-migration"
	plain.ConnectionVersion = "pre-migration"
	if err = writeProfile(ctx, dir, "work", plain); err != nil {
		t.Fatal(err)
	}
	if _, err = OpenProfile(s.Directory(), "work"); !errors.Is(err, ErrMigrationPending) {
		t.Fatal("ambiguous migration was accepted")
	}
	if err = s.EncryptPlaintext(ctx, []byte("wrong sufficiently long passphrase")); !errors.Is(err, ErrUnlock) {
		t.Fatal("wrong recovery passphrase accepted")
	}
	if _, err = os.Stat(filepath.Join(s.Directory(), "work.json")); err != nil {
		t.Fatal("failed recovery removed plaintext")
	}
	changed := plain
	changed.OpenAIAPIKey = "different"
	changed.OpenAIOAuth = nil
	if err = writeProfile(ctx, dir, "work", changed); err != nil {
		t.Fatal(err)
	}
	if err = s.EncryptPlaintext(ctx, []byte("correct horse battery staple")); !errors.Is(err, ErrConflict) {
		t.Fatal("different credentials were silently removed")
	}
	if err = writeProfile(ctx, dir, "work", plain); err != nil {
		t.Fatal(err)
	}
	if err = s.EncryptPlaintext(ctx, []byte("correct horse battery staple")); err != nil {
		t.Fatal(err)
	}
}

type memoryKeyring struct {
	keys        map[string][]byte
	unavailable bool
}

func (k *memoryKeyring) Put(_ context.Context, id string, key []byte) error {
	if k.unavailable {
		return ErrLocked
	}
	k.keys[id] = append([]byte{}, key...)
	return nil
}
func (k *memoryKeyring) Get(_ context.Context, id string) ([]byte, error) {
	if k.unavailable {
		return nil, ErrLocked
	}
	return append([]byte{}, k.keys[id]...), nil
}
func (k *memoryKeyring) Delete(_ context.Context, id string) error { delete(k.keys, id); return nil }
func TestEncryptedKeyringHasNoFallback(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "profile")
	s, err := NewEncryptedStore(dir, "work")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	ring := &memoryKeyring{keys: map[string][]byte{}}
	if err = s.CreateWithKeyring(ctx, "work", ring, Credentials{AnthropicAPIKey: "fake-keyring-api-key"}); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	if err = s.Unlock(ctx, []byte("not a passphrase profile")); !errors.Is(err, ErrUnlock) {
		t.Fatal("keyring backend accepted passphrase")
	}
	ring.unavailable = true
	if err = s.UnlockWithKeyring(ctx, ring); err == nil {
		t.Fatal("unavailable backend accepted")
	}
	if _, err = s.Load(ctx, "work"); !errors.Is(err, ErrLocked) {
		t.Fatal("unavailable keyring fell back to unlocked storage")
	}
	ring.unavailable = false
	if err = s.UnlockWithKeyring(ctx, ring); err != nil {
		t.Fatal(err)
	}
	value, err := s.Load(ctx, "work")
	if err != nil || value.AnthropicAPIKey != "fake-keyring-api-key" {
		t.Fatal("keyring lost credentials", err)
	}
}

func BenchmarkEncryptedUnlock(b *testing.B) {
	s, err := NewEncryptedStore(filepath.Join(b.TempDir(), "profile"), "work")
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	pass := []byte("correct horse battery staple")
	if err = s.CreateWithPassphrase(context.Background(), "work", pass, Credentials{OpenAIAPIKey: "fake"}); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err = s.Unlock(context.Background(), pass); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkEncryptedRefresh(b *testing.B) {
	s, err := NewEncryptedStore(filepath.Join(b.TempDir(), "profile"), "work")
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if err = s.CreateWithPassphrase(context.Background(), "work", []byte("correct horse battery staple"), Credentials{OpenAIOAuth: &OpenAIOAuth{AccessToken: "fake", RefreshToken: "fake-refresh"}}); err != nil {
		b.Fatal(err)
	}
	value, err := s.Load(context.Background(), "work")
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		value.Version, err = s.SaveRefresh(context.Background(), "work", value.Version, "openai", value)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncryptedLoad(b *testing.B) {
	s, err := NewEncryptedStore(filepath.Join(b.TempDir(), "profile"), "work")
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if err = s.CreateWithPassphrase(context.Background(), "work", []byte("correct horse battery staple"), Credentials{OpenAIAPIKey: "fixture-load-benchmark"}); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err = s.Load(context.Background(), "work"); err != nil {
			b.Fatal(err)
		}
	}
}
