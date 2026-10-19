// Package workerapp owns the host-side launch lifecycle shared by the CLI and
// embedders. Interactive prompts and process signals belong to the caller.
package workerapp

import (
	"context"
	"fmt"

	"github.com/pacificbelt30/codex-dock/internal/sandbox"
	"github.com/pacificbelt30/codex-dock/internal/worktree"
)

type Runner interface {
	RunContext(context.Context, sandbox.RunOptions) (string, error)
}

// Launch creates an optional worktree and hands its cleanup to the sandbox
// manager once a worker has been created. A failed launch without a retained
// worker removes the temporary worktree.
func Launch(ctx context.Context, runner Runner, opts sandbox.RunOptions) (id string, runErr error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if opts.UseWorktree && opts.WorktreePath == "" {
		path, err := worktree.CreateContext(ctx, worktree.CreateOptions{
			ProjectDir: opts.ProjectDir,
			Branch:     opts.Branch,
			NewBranch:  opts.NewBranch,
		})
		if err != nil && opts.ReuseBranchOnRetry && opts.NewBranch && ctx.Err() == nil {
			path, err = worktree.CreateContext(ctx, worktree.CreateOptions{
				ProjectDir: opts.ProjectDir,
				Branch:     opts.Branch,
			})
		}
		if err != nil {
			return "", fmt.Errorf("creating worktree: %w", err)
		}
		opts.WorktreePath = path
		defer func() {
			if id != "" {
				return // Manager.Remove now owns worktree cleanup.
			}
			if err := worktree.Remove(path); err != nil {
				runErr = fmt.Errorf("%w; removing worktree: %v", runErr, err)
			}
		}()
	}
	return runner.RunContext(ctx, opts)
}
