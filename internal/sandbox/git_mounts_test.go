package sandbox

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
)

func gitFixture(t *testing.T) (string, string) {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "repo")
	wt := filepath.Join(t.TempDir(), "linked")
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git fixture: %v: %s", err, out)
		}
	}
	git("init", repo)
	git("-C", repo, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-m", "fixture")
	git("-C", repo, "worktree", "add", "-b", "fixture-worker", wt)
	return repo, wt
}

func TestKataLinkedWorktreePreservesPointerAndLifetime(t *testing.T) {
	repo, wt := gitFixture(t)
	pointer := filepath.Join(wt, ".git")
	before, err := os.ReadFile(pointer)
	if err != nil {
		t.Fatal(err)
	}
	f, m := newEngineFixture(t)
	id, err := m.Run(RunOptions{Name: "worker", ProjectDir: repo, WorktreePath: wt, UseWorktree: true, Detach: true})
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(pointer)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(before, after) {
		t.Fatal("host .git pointer changed during preparation")
	}
	common := filepath.Join(repo, ".git")
	found := false
	for _, mount := range f.worker.HostConfig.Mounts {
		if mount.Target == "/workspace/.git" {
			t.Fatal("nested .git file mount can corrupt a host pointer")
		}
		if mount.Source == common && mount.Target == common {
			found = true
		}
	}
	if !found {
		t.Fatal("linked worktree metadata not available in guest")
	}
	if value, _ := envValue(f.worker.Config.Env, "GIT_DIR"); value == "" {
		t.Fatal("canonical Git directory not configured")
	}
	if err := m.Remove(id, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Fatalf("removed worker worktree still exists: %v", err)
	}
	if _, err := os.Stat(common); err != nil {
		t.Fatal("main repository was removed")
	}
}

func TestLinkedWorktreeReadOnlyMetadata(t *testing.T) {
	_, wt := gitFixture(t)
	mounts, _, err := linkedWorktreeMounts(context.Background(), wt, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(mounts) != 1 || !mounts[0].ReadOnly {
		t.Fatal("review workspace received writable Git metadata")
	}
}
