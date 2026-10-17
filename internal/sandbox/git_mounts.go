package sandbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
)

// A linked worktree's .git points outside /workspace. Preserve that pointer
// without adding a nested mount over .git (Kata prepares file mountpoints
// differently from runc). The common metadata keeps its absolute host path.
// This retains normal Git commit/branch behavior; it is not a private snapshot
// of shared repository metadata.
func linkedWorktreeMounts(ctx context.Context, workspace string, readOnly bool, existing []mount.Mount) ([]mount.Mount, []string, error) {
	info, err := os.Lstat(filepath.Join(workspace, ".git"))
	if os.IsNotExist(err) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	if info.IsDir() {
		return nil, nil, nil
	}
	if !info.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("workspace .git must be a regular pointer file or directory")
	}
	cmd := exec.CommandContext(ctx, "git", "-C", workspace, "rev-parse", "--path-format=absolute", "--git-common-dir")
	data, err := cmd.Output()
	if err != nil {
		return nil, nil, fmt.Errorf("resolving linked worktree metadata: %w", err)
	}
	common := strings.TrimSpace(string(data))
	common, err = filepath.EvalSymlinks(common)
	if err != nil {
		return nil, nil, err
	}
	if !filepath.IsAbs(common) || common == "/" {
		return nil, nil, fmt.Errorf("invalid linked worktree metadata directory")
	}
	gitDirBytes, err := exec.CommandContext(ctx, "git", "-C", workspace, "rev-parse", "--absolute-git-dir").Output()
	if err != nil {
		return nil, nil, fmt.Errorf("resolving worktree Git directory: %w", err)
	}
	gitDir, err := filepath.EvalSymlinks(strings.TrimSpace(string(gitDirBytes)))
	if err != nil {
		return nil, nil, err
	}
	rel, err := filepath.Rel(common, gitDir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, nil, fmt.Errorf("worktree Git directory is outside its common metadata")
	}
	for _, m := range existing {
		if m.Target == common {
			return nil, nil, fmt.Errorf("mount target conflicts with linked worktree metadata: %s", common)
		}
	}
	return []mount.Mount{{Type: mount.TypeBind, Source: common, Target: common, ReadOnly: readOnly,
		BindOptions: &mount.BindOptions{Propagation: mount.PropagationRPrivate}}}, []string{"GIT_DIR=" + gitDir, "GIT_WORK_TREE=/workspace"}, nil
}

// Remove only worktrees created by the CLI, after the container and network
// have been removed. Never fall back to recursively deleting an unknown path.
func cleanupManagedWorktree(ctx context.Context, v container.InspectResponse) error {
	path := v.Config.Labels[labelPrefix+"worktree"]
	if path == "" {
		return nil
	}
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || path == "/" {
		return fmt.Errorf("invalid managed worktree path")
	}
	matched := false
	for _, m := range v.HostConfig.Mounts {
		if m.Target == "/workspace" && m.Source == path {
			matched = true
		}
	}
	if !matched {
		return fmt.Errorf("managed worktree does not match the worker workspace")
	}
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return nil
	}
	info, err := os.Lstat(filepath.Join(path, ".git"))
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("managed worktree %s has no regular .git pointer; retained for recovery", path)
	}
	cmd := exec.CommandContext(ctx, "git", "-C", path, "worktree", "remove", "--force", "--", path)
	if data, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("removing managed worktree %s: %w: %s", path, err, strings.TrimSpace(string(data)))
	}
	return nil
}
