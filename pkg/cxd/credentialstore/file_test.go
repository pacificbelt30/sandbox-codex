package credentialstore_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/pacificbelt30/codex-dock/pkg/cxd/credentialstore"
	"github.com/pacificbelt30/codex-dock/pkg/cxd/credentialstore/storetest"
)

type failingFileStore struct {
	credentialstore.Store
	fail atomic.Bool
}

func (s *failingFileStore) SaveRefresh(ctx context.Context, id, version, provider string, next credentialstore.Credentials) (string, error) {
	if s.fail.Swap(false) {
		return "", errors.New("injected pre-commit failure")
	}
	return s.Store.SaveRefresh(ctx, id, version, provider, next)
}

func fileHarness(t *testing.T) storetest.Harness {
	t.Helper()
	directory := filepath.Join(t.TempDir(), "profiles")
	store, err := credentialstore.NewFileStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	failing := &failingFileStore{Store: store}
	return storetest.Harness{Store: failing,
		// Only the fixture controls may seed explicit generations. Production
		// Create generates new versions and never overwrites an existing login.
		Replace: func(ctx context.Context, id string, v credentialstore.Credentials) error {
			data, err := json.Marshal(map[string]any{"schema": 1, "name": id, "credentials": v})
			if err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(directory, id+".json"), data, 0600)
		},
		Delete:       func(ctx context.Context, id string) error { return os.Remove(filepath.Join(directory, id+".json")) },
		Reopen:       func(context.Context) (credentialstore.Store, error) { return credentialstore.NewFileStore(directory) },
		FailNextSave: func(context.Context) error { failing.fail.Store(true); return nil },
	}
}

func TestFileStoreContract(t *testing.T) { storetest.Run(t, fileHarness) }

func TestFileStorePermissionsAndIsolation(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "profiles")
	store, err := credentialstore.NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(ctx, "first", credentialstore.Credentials{OpenAIAPIKey: "fixture-first"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Create(ctx, "second", credentialstore.Credentials{AnthropicAPIKey: "fixture-second"}); err != nil {
		t.Fatal(err)
	}
	first, _ := store.Load(ctx, "first")
	second, _ := store.Load(ctx, "second")
	if first.OpenAIAPIKey != "fixture-first" || first.AnthropicAPIKey != "" || second.OpenAIAPIKey != "" || second.AnthropicAPIKey != "fixture-second" || first.Version == second.Version || first.ConnectionVersion == second.ConnectionVersion {
		t.Fatal("profiles or generations were mixed")
	}
	if err := store.Create(ctx, "first", second); err == nil {
		t.Fatal("Create silently replaced existing credentials")
	}
	for _, name := range []string{"first.json", "second.json", ".lock"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatal("profile or lock permissions are not 0600")
		}
	}
	if info, _ := os.Stat(dir); info.Mode().Perm() != 0700 {
		t.Fatal("profile directory is not 0700")
	}
	for _, name := range []string{"../first", "", "a/b", "UPPER", ".lock"} {
		if _, err := store.Load(ctx, name); err == nil {
			t.Fatal("unsafe profile name accepted")
		}
	}
	if err := os.Chmod(filepath.Join(dir, "first.json"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(ctx, "first"); err == nil {
		t.Fatal("world-readable credential accepted")
	}
	if err := os.Symlink(filepath.Join(dir, "second.json"), filepath.Join(dir, "alias.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(ctx, "alias"); err == nil {
		t.Fatal("symlink credential accepted")
	}
	if _, err := store.Load(ctx, "missing"); !errors.Is(err, credentialstore.ErrNotFound) {
		t.Fatal("missing profile did not fail closed")
	}
}

func TestFileStoreActualWriteFailureAndWrongProvider(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "profiles")
	store, err := credentialstore.NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(ctx, "test", credentialstore.Credentials{OpenAIOAuth: &credentialstore.OpenAIOAuth{AccessToken: "access", RefreshToken: "refresh"}}); err != nil {
		t.Fatal(err)
	}
	before, _ := store.Load(ctx, "test")
	next, _ := store.Load(ctx, "test")
	next.OpenAIOAuth.AccessToken = "rotated"
	if _, err := store.SaveRefresh(ctx, "test", before.Version, "anthropic", next); err == nil {
		t.Fatal("refresh for another provider accepted")
	}
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatal(err)
	}
	_, saveErr := store.SaveRefresh(ctx, "test", before.Version, "openai", next)
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	after, err := store.Load(ctx, "test")
	if saveErr == nil || err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("failed filesystem write changed acknowledged credentials")
	}
}

func TestFileStoreProcessCAS(t *testing.T) {
	ctx := context.Background()
	if os.Getenv("CXD_FILESTORE_CHILD") == "1" {
		store, err := credentialstore.NewFileStore(os.Getenv("CXD_FILESTORE_DIR"))
		if err != nil {
			t.Fatal(err)
		}
		next, err := store.Load(ctx, "test")
		if err != nil {
			t.Fatal(err)
		}
		fmt.Println("ready")
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
		_, err = store.SaveRefresh(ctx, "test", next.Version, "openai", next)
		if err == nil {
			fmt.Println("winner")
		} else if !errors.Is(err, credentialstore.ErrConflict) {
			t.Fatal("unexpected CAS failure")
		}
		return
	}
	dir := filepath.Join(t.TempDir(), "profiles")
	store, err := credentialstore.NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(ctx, "test", credentialstore.Credentials{OpenAIOAuth: &credentialstore.OpenAIOAuth{AccessToken: "fixture", RefreshToken: "fixture"}}); err != nil {
		t.Fatal(err)
	}
	var processes []*exec.Cmd
	var readers []*bufio.Reader
	var release []func()
	for range 2 {
		command := exec.Command(os.Args[0], "-test.run=^TestFileStoreProcessCAS$")
		command.Env = []string{"CXD_FILESTORE_CHILD=1", "CXD_FILESTORE_DIR=" + dir}
		input, err := command.StdinPipe()
		if err != nil {
			t.Fatal(err)
		}
		output, err := command.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
		reader := bufio.NewReader(output)
		if line, err := reader.ReadString('\n'); err != nil || line != "ready\n" {
			t.Fatal("child did not reach CAS barrier")
		}
		processes, readers = append(processes, command), append(readers, reader)
		release = append(release, func() { _, _ = fmt.Fprintln(input, "go"); _ = input.Close() })
	}
	for _, run := range release {
		run()
	}
	winners := 0
	for i, command := range processes {
		for {
			line, err := readers[i].ReadString('\n')
			if strings.TrimSpace(line) == "winner" {
				winners++
			}
			if err != nil {
				break
			}
		}
		if err := command.Wait(); err != nil {
			t.Fatal(err)
		}
	}
	if winners != 1 {
		t.Fatal("cross-process CAS did not have exactly one winner")
	}
}
