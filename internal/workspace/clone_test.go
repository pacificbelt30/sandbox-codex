package workspace

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCloneIndependentObjectsAndWorkspace(t *testing.T) {
	ctx := context.Background()
	src := t.TempDir()
	git := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %q: %v %s", args, err, out)
		}
	}
	git(src, "init", "-q")
	git(src, "config", "user.name", "fixture")
	git(src, "config", "user.email", "fixture@example.invalid")
	if err := os.WriteFile(filepath.Join(src, "tracked"), []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	git(src, "add", "tracked")
	git(src, "commit", "-qm", "initial")
	dest := filepath.Join(t.TempDir(), "copy")
	if err := Clone(ctx, src, dest, "task", true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, ".git", "objects", "info", "alternates")); !os.IsNotExist(err) {
		t.Fatal("clone shares object alternates")
	}
	if err := os.WriteFile(filepath.Join(dest, "tracked"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(src, "tracked")); string(got) != "original" {
		t.Fatal("source changed")
	}
	if err := Clone(ctx, src, dest, "", false); err == nil {
		t.Fatal("adopted existing checkout")
	}
}
