package sandbox

import (
	"github.com/pacificbelt30/codex-dock/internal/workerpolicy"
	"os"
	"path/filepath"
	"testing"
)

func TestFilesystemTargetsRequireExistingContainedPaths(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "secret"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateFilesystemTargets(root, workerpolicy.Filesystem{DenyRead: []string{"/workspace/secret"}}); err != nil {
		t.Fatal(err)
	}
	if err := validateFilesystemTargets(root, workerpolicy.Filesystem{DenyRead: []string{"/workspace/future"}}); err == nil {
		t.Fatal("missing mask target accepted")
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "outside")); err != nil {
		t.Fatal(err)
	}
	if err := validateFilesystemTargets(root, workerpolicy.Filesystem{DenyWrite: []string{"/workspace/outside"}}); err == nil {
		t.Fatal("symlink mask accepted")
	}
}
