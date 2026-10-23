package credentialstore_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/pacificbelt30/codex-dock/pkg/cxd/credentialstore"
	"github.com/pacificbelt30/codex-dock/pkg/cxd/credentialstore/storetest"
)

type encryptedRouter map[string]*credentialstore.EncryptedStore

func (s encryptedRouter) Load(ctx context.Context, id string) (credentialstore.Credentials, error) {
	if v := s[id]; v != nil {
		return v.Load(ctx, id)
	}
	return credentialstore.Credentials{}, credentialstore.ErrNotFound
}
func (s encryptedRouter) SaveRefresh(ctx context.Context, id, version, provider string, v credentialstore.Credentials) (string, error) {
	if store := s[id]; store != nil {
		return store.SaveRefresh(ctx, id, version, provider, v)
	}
	return "", credentialstore.ErrNotFound
}
func TestEncryptedStoreContract(t *testing.T) {
	storetest.Run(t, func(t *testing.T) storetest.Harness {
		dir := filepath.Join(t.TempDir(), "profile")
		stores := encryptedRouter{}
		failing := &failingFileStore{Store: stores}
		register := func(s *credentialstore.EncryptedStore) { t.Cleanup(func() { _ = s.Close() }) }
		return storetest.Harness{Store: failing,
			Replace: func(ctx context.Context, id string, v credentialstore.Credentials) error {
				s := stores[id]
				if s == nil {
					var err error
					s, err = credentialstore.NewEncryptedStore(dir, id)
					if err != nil {
						return err
					}
					register(s)
					if err = s.CreateWithPassphrase(ctx, id, []byte("fixture correct long passphrase"), v); err != nil {
						return err
					}
					stores[id] = s
				}
				return credentialstore.WriteEncryptedFixture(ctx, s, v)
			},
			Delete: func(_ context.Context, id string) error { return os.Remove(filepath.Join(dir, id+".vault")) },
			Reopen: func(ctx context.Context) (credentialstore.Store, error) {
				result := encryptedRouter{}
				for id, s := range stores {
					fresh, err := credentialstore.NewEncryptedStore(dir, id)
					if err != nil {
						return nil, err
					}
					register(fresh)
					key, err := s.UnlockedKey()
					if err != nil {
						return nil, err
					}
					err = fresh.UnlockWithKey(ctx, key)
					if errors.Is(err, credentialstore.ErrNotFound) {
						err = nil
					}
					clear(key)
					if err != nil {
						return nil, err
					}
					result[id] = fresh
				}
				return result, nil
			},
			FailNextSave: func(context.Context) error { failing.fail.Store(true); return nil },
		}
	})
}
