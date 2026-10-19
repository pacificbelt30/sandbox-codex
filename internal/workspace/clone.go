// Package workspace prepares independent local Git copies. Clones are retained
// for recovery and are never deleted by worker removal or lifecycle retention.
package workspace

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func Clone(ctx context.Context, source, destination, branch string, newBranch bool) error {
	if !filepath.IsAbs(destination) || filepath.Clean(destination) != destination {
		return fmt.Errorf("clone directory must be a clean absolute path")
	}
	if branch != "" {
		cmd := exec.CommandContext(ctx, "git", "check-ref-format", "--branch", branch)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("invalid clone branch")
		}
	}
	// Reserve a new directory atomically; never replace an existing checkout.
	if err := os.Mkdir(destination, 0700); err != nil {
		return err
	}
	complete := false
	defer func() {
		if !complete {
			_ = os.RemoveAll(destination)
		}
	}()
	cmd := exec.CommandContext(ctx, "git", "clone", "--no-local", "--no-hardlinks", "--", source, destination)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("creating independent clone: %w: %s", err, out)
	}
	if branch != "" {
		args := []string{"-C", destination, "checkout"}
		if newBranch {
			args = append(args, "-b")
		}
		args = append(args, branch, "--")
		if out, err := exec.CommandContext(ctx, "git", args...).CombinedOutput(); err != nil {
			return fmt.Errorf("selecting clone branch: %w: %s", err, out)
		}
	}
	complete = true
	return nil
}
