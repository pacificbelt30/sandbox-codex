package workerapp

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/pacificbelt30/codex-dock/internal/sandbox"
)

type fakeRunner struct {
	id   string
	err  error
	opts sandbox.RunOptions
}

func (f *fakeRunner) RunContext(_ context.Context, opts sandbox.RunOptions) (string, error) {
	f.opts = opts
	return f.id, f.err
}

func TestLaunchWorktreeOwnershipOnFailure(t *testing.T) {
	project := filepath.Join(t.TempDir(), "project")
	if err := os.Mkdir(project, 0700); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-b", "main"}, {"config", "user.email", "test@example.com"}, {"config", "user.name", "Test"}, {"commit", "--allow-empty", "-m", "init"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = project
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	opts := sandbox.RunOptions{ProjectDir: project, UseWorktree: true, Branch: "api-test", NewBranch: true}
	path := project + "-dock-api-test"
	f := &fakeRunner{err: errors.New("create failed")}
	if _, err := Launch(context.Background(), f, opts); err == nil {
		t.Fatal("expected launch failure")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("unowned worktree remains: %v", err)
	}
	if f.opts.WorktreePath != path {
		t.Fatalf("runner worktree = %q", f.opts.WorktreePath)
	}
	f.id = "worker-id"
	opts.NewBranch = false
	if id, err := Launch(context.Background(), f, opts); id != f.id || err == nil {
		t.Fatalf("retained worker result = %q, %v", id, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("retained worker worktree removed: %v", err)
	}
}

func TestLaunchCancelledBeforeWorktree(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f := &fakeRunner{}
	if _, err := Launch(ctx, f, sandbox.RunOptions{UseWorktree: true}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled launch = %v", err)
	}
}
