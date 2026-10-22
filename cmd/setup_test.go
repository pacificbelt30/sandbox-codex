package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupBundleBesideExecutable(t *testing.T) {
	dir := t.TempDir()
	cli := filepath.Join(dir, "oaiso")
	for _, path := range []string{cli, filepath.Join(dir, "bundle.json")} {
		if err := os.WriteFile(path, []byte("test"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(t.TempDir(), "oaiso")
	if err := os.Symlink(cli, link); err != nil {
		t.Fatal(err)
	}
	got, err := setupBundleBeside(link)
	if err != nil || got != dir {
		t.Fatalf("setupBundleBeside(%s) = %q, %v; want %q", link, got, err, dir)
	}
	if err := os.Remove(filepath.Join(dir, "bundle.json")); err != nil {
		t.Fatal(err)
	}
	_, err = setupBundleBeside(link)
	if err == nil || !strings.Contains(err.Error(), "--bundle PATH") {
		t.Fatalf("expected explicit bundle guidance, got %v", err)
	}
}

func TestSetupOperatorRequiresExplicitRootGrant(t *testing.T) {
	if got, err := setupOperator("alice", true); err != nil || got != "alice" {
		t.Fatalf("explicit operator = %q, %v", got, err)
	}
	if _, err := setupOperator("", true); err == nil || !strings.Contains(err.Error(), "--operator USER") {
		t.Fatalf("expected explicit root grant guidance, got %v", err)
	}
}
