package storetest_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/pacificbelt30/codex-dock/pkg/cxd/credentialstore"
	"github.com/pacificbelt30/codex-dock/pkg/cxd/credentialstore/storetest"
)

// This disk fixture exercises the suite itself. It is not a production Store:
// its shared mutex coordinates only adapters in the same test process.
type database struct {
	mu   sync.Mutex
	path string
	fail bool
}
type adapter struct{ db *database }

func (a *adapter) read() (map[string]credentialstore.Credentials, error) {
	values := map[string]credentialstore.Credentials{}
	b, err := os.ReadFile(a.db.path)
	if os.IsNotExist(err) {
		return values, nil
	}
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(b, &values)
	return values, err
}
func (a *adapter) write(values map[string]credentialstore.Credentials) error {
	b, err := json.Marshal(values)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(a.db.path), ".save-")
	if err != nil {
		return err
	}
	defer func() { _ = f.Close(); _ = os.Remove(f.Name()) }()
	if _, err = f.Write(b); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), a.db.path); err != nil {
		return err
	}
	d, err := os.Open(filepath.Dir(a.db.path))
	if err != nil {
		return err
	}
	defer func() { _ = d.Close() }()
	return d.Sync()
}
func (a *adapter) Load(ctx context.Context, id string) (credentialstore.Credentials, error) {
	a.db.mu.Lock()
	defer a.db.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return credentialstore.Credentials{}, err
	}
	values, err := a.read()
	if err != nil {
		return credentialstore.Credentials{}, err
	}
	v, ok := values[id]
	if !ok {
		return v, credentialstore.ErrNotFound
	}
	return v, nil
}
func (a *adapter) SaveRefresh(ctx context.Context, id, version, provider string, next credentialstore.Credentials) (string, error) {
	a.db.mu.Lock()
	defer a.db.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	values, err := a.read()
	if err != nil {
		return "", err
	}
	v, ok := values[id]
	if !ok {
		return "", credentialstore.ErrNotFound
	}
	if v.Version != version {
		return "", credentialstore.ErrConflict
	}
	if provider != "openai" && provider != "anthropic" {
		return "", errors.New("unsupported provider")
	}
	if a.db.fail {
		a.db.fail = false
		return "", errors.New("injected durable write failure")
	}
	next.Version = version + "-next"
	values[id] = next
	if err := a.write(values); err != nil {
		return "", err
	}
	return next.Version, nil
}
func newHarness(t *testing.T) storetest.Harness {
	t.Helper()
	a := &adapter{db: &database{path: filepath.Join(t.TempDir(), "state.json")}}
	return storetest.Harness{Store: a,
		Replace: func(ctx context.Context, id string, v credentialstore.Credentials) error {
			a.db.mu.Lock()
			defer a.db.mu.Unlock()
			if err := ctx.Err(); err != nil {
				return err
			}
			values, err := a.read()
			if err != nil {
				return err
			}
			values[id] = v
			return a.write(values)
		},
		Delete: func(ctx context.Context, id string) error {
			a.db.mu.Lock()
			defer a.db.mu.Unlock()
			if err := ctx.Err(); err != nil {
				return err
			}
			values, err := a.read()
			if err != nil {
				return err
			}
			delete(values, id)
			return a.write(values)
		},
		Reopen:       func(context.Context) (credentialstore.Store, error) { return &adapter{db: a.db}, nil },
		FailNextSave: func(context.Context) error { a.db.mu.Lock(); defer a.db.mu.Unlock(); a.db.fail = true; return nil },
	}
}
func TestDiskAdapterConformance(t *testing.T) { storetest.Run(t, newHarness) }

type brokenAdapter struct {
	credentialstore.Store
	mode    string
	mu      sync.Mutex
	replace func(context.Context, string, credentialstore.Credentials) error
}

func (a *brokenAdapter) SaveRefresh(ctx context.Context, id, version, provider string, next credentialstore.Credentials) (string, error) {
	switch a.mode {
	case "stale-success":
		v, err := a.Load(ctx, id)
		if err == nil && v.Version != version {
			return v.Version, nil
		}
	case "volatile-save":
		if _, err := a.Load(ctx, id); err != nil {
			return "", err
		}
		return version + "-volatile", nil
	case "connection-bump":
		next.ConnectionVersion += "-refresh"
	case "multiple-winners":
		a.mu.Lock()
		defer a.mu.Unlock()
		v, err := a.Load(ctx, id)
		if err != nil {
			return "", err
		}
		version = v.Version
	}
	newVersion, err := a.Store.SaveRefresh(ctx, id, version, provider, next)
	if a.mode == "publish-on-failure" && err != nil && !errors.Is(err, credentialstore.ErrNotFound) && !errors.Is(err, credentialstore.ErrConflict) && !errors.Is(err, context.Canceled) {
		_ = a.replace(ctx, id, next)
	}
	return newVersion, err
}

// Confirm that the reusable suite rejects real contract violations instead of
// merely exercising happy paths. Child failures must not fail the parent run.
func TestContractRejectsBrokenAdapters(t *testing.T) {
	if mode := os.Getenv("CXD_STORETEST_MUTANT"); mode != "" {
		storetest.Run(t, func(t *testing.T) storetest.Harness {
			h := newHarness(t)
			h.Store = &brokenAdapter{Store: h.Store, mode: mode, replace: h.Replace}
			return h
		})
		return
	}
	for _, mode := range []string{"stale-success", "volatile-save", "connection-bump", "multiple-winners", "publish-on-failure"} {
		t.Run(mode, func(t *testing.T) {
			cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestContractRejectsBrokenAdapters$", "-test.timeout=20s")
			cmd.Env = append(os.Environ(), "CXD_STORETEST_MUTANT="+mode)
			if err := cmd.Run(); err == nil {
				t.Fatal("conformance suite accepted a broken adapter")
			} else {
				var exit *exec.ExitError
				if !errors.As(err, &exit) || exit.ExitCode() != 1 {
					t.Fatalf("child conformance process failed unexpectedly: %v", err)
				}
			}
		})
	}
}
