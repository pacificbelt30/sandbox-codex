package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/pacificbelt30/codex-dock/internal/localkeyring"
	"github.com/pacificbelt30/codex-dock/pkg/cxd/credentialstore"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type profileCacheKey struct{}
type profileCache struct {
	mu      sync.Mutex
	stores  map[string]*credentialstore.EncryptedStore
	command *cobra.Command
}

func newProfileCache(cmd *cobra.Command) *profileCache {
	return &profileCache{stores: make(map[string]*credentialstore.EncryptedStore), command: cmd}
}
func (c *profileCache) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, s := range c.stores {
		_ = s.Close()
	}
	clear(c.stores)
}
func readPassphrase(cmd *cobra.Command, confirmation bool) ([]byte, error) {
	// The numeric FD is public configuration; its contents never enter argv/env.
	if flag := rootCmd.PersistentFlags().Lookup("passphrase-fd"); flag != nil && flag.Changed {
		fd, err := strconv.Atoi(flag.Value.String())
		if err != nil {
			return nil, fmt.Errorf("invalid passphrase descriptor")
		}
		return readPassphraseFD(fd)
	}
	return readTerminalPassphrase(cmd, confirmation)
}
func readPassphraseFD(fd int) ([]byte, error) {
	if fd < 3 {
		return nil, fmt.Errorf("passphrase descriptor must be >= 3")
	}
	f := os.NewFile(uintptr(fd), "oaiso-passphrase")
	if f == nil {
		return nil, fmt.Errorf("invalid passphrase descriptor")
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, 4097))
	if err != nil || len(data) > 4096 {
		clear(data)
		return nil, fmt.Errorf("passphrase descriptor could not be read (maximum 4096 bytes)")
	}
	return bytes.TrimSuffix(data, []byte{'\n'}), nil
}
func readTerminalPassphrase(cmd *cobra.Command, confirmation bool) ([]byte, error) {
	f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil || !term.IsTerminal(int(f.Fd())) {
		if f != nil {
			_ = f.Close()
		}
		return nil, fmt.Errorf("encrypted profile needs a terminal or --passphrase-fd (no host credential fallback)")
	}
	defer func() { _ = f.Close() }()
	_, _ = fmt.Fprint(cmd.ErrOrStderr(), "Passphrase: ")
	pass, err := term.ReadPassword(int(f.Fd()))
	_, _ = fmt.Fprintln(cmd.ErrOrStderr())
	if err != nil {
		return nil, fmt.Errorf("reading passphrase failed")
	}
	if confirmation {
		_, _ = fmt.Fprint(cmd.ErrOrStderr(), "Repeat passphrase: ")
		again, e := term.ReadPassword(int(f.Fd()))
		_, _ = fmt.Fprintln(cmd.ErrOrStderr())
		defer clear(again)
		if e != nil || !bytes.Equal(pass, again) {
			clear(pass)
			return nil, fmt.Errorf("passphrase confirmation did not match")
		}
	}
	return pass, nil
}
func unlockedAuthProfile(ctx context.Context, name string) (credentialstore.ProfileStore, error) {
	store, err := openAuthProfile(name)
	if err != nil {
		return nil, err
	}
	encrypted, ok := store.(*credentialstore.EncryptedStore)
	if !ok {
		return store, nil
	}
	cache, ok := ctx.Value(profileCacheKey{}).(*profileCache)
	if !ok {
		return nil, credentialstore.ErrLocked
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	cacheID := store.Directory() + "\x00" + name
	if existing := cache.stores[cacheID]; existing != nil {
		return existing, nil
	}
	info, err := encrypted.Info(ctx)
	if err != nil {
		return nil, err
	}
	if info.Format != "age" {
		return nil, credentialstore.ErrMigrationRequired
	}
	if info.Backend == "secret-service" {
		err = encrypted.UnlockWithKeyring(ctx, localkeyring.SecretService{})
	} else {
		pass, e := readPassphrase(cache.command, false)
		if e != nil {
			return nil, e
		}
		err = encrypted.Unlock(ctx, pass)
		clear(pass)
	}
	if err != nil {
		_ = encrypted.Close()
		return nil, err
	}
	cache.stores[cacheID] = encrypted
	return encrypted, nil
}

// Once the proxy owns its key, a long-running host agent command need not keep
// another unlocked copy. Later mode checks reuse authenticated proxy metadata.
func forgetUnlockedProfile(ctx context.Context, name string) {
	cache, ok := ctx.Value(profileCacheKey{}).(*profileCache)
	if !ok {
		return
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	for id, s := range cache.stores {
		if strings.HasSuffix(id, "\x00"+name) {
			_ = s.Close()
			delete(cache.stores, id)
		}
	}
}
