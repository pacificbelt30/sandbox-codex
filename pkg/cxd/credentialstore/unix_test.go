package credentialstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type memoryStore struct {
	mu    sync.Mutex
	items map[string]Credentials
}

func (s *memoryStore) Load(_ context.Context, id string) (Credentials, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.items[id]
	if !ok {
		return Credentials{}, ErrNotFound
	}
	return v, nil
}

func (s *memoryStore) SaveRefresh(_ context.Context, id, version, provider string, next Credentials) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.items[id]
	if !ok {
		return "", ErrNotFound
	}
	if current.Version != version {
		return "", ErrConflict
	}
	if provider != "openai" && provider != "anthropic" {
		return "", errors.New("invalid provider")
	}
	next.Version = version + "-next"
	s.items[id] = next
	return next.Version, nil
}

func TestUnixStoreIsPrivateAndConfinedToCase(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "store.sock")
	store := &memoryStore{items: map[string]Credentials{
		"user-a": {Version: "a1", OpenAIAPIKey: "key-a"},
		"user-b": {Version: "b1", OpenAIAPIKey: "key-b"},
	}}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, path, "user-a", store) }()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
		}
	}()
	deadline := time.Now().Add(time.Second)
	for {
		if _, err := os.Lstat(path); err == nil {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("store stopped before creating socket: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("store socket was not created")
		}
		time.Sleep(time.Millisecond)
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("socket permissions = %v, %v", info, err)
	}
	client, err := NewUnixClient(path)
	if err != nil {
		t.Fatal(err)
	}
	a, err := client.Load(context.Background(), "user-a")
	if err != nil || a.OpenAIAPIKey != "key-a" {
		t.Fatalf("A credentials = %+v, %v", a, err)
	}
	if _, err := client.SaveRefresh(context.Background(), "user-a", "stale", "openai", a); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale save = %v", err)
	}
	a.OpenAIAPIKey = "rotated-a"
	if _, err := client.SaveRefresh(context.Background(), "user-a", "a1", "openai", a); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Load(context.Background(), "user-b"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("B credentials were exposed: %v", err)
	}
	if _, err := client.SaveRefresh(context.Background(), "user-b", "b1", "openai", Credentials{OpenAIAPIKey: "changed"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("B credentials were writable: %v", err)
	}
	if b, err := store.Load(context.Background(), "user-b"); err != nil || b.OpenAIAPIKey != "key-b" || b.Version != "b1" {
		t.Fatalf("B credentials changed: %+v, %v", b, err)
	}
}
