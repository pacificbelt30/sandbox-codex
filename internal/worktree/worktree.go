package worktree

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// CreateOptions holds options for creating a git worktree.
type CreateOptions struct {
	ProjectDir string
	Branch     string
	NewBranch  bool
}

// Create creates a git worktree and returns its path.
// The worktree is created at ${project}/../${project_name}-dock-${branch}.
func Create(opts CreateOptions) (string, error) {
	if opts.Branch == "" {
		return "", fmt.Errorf("branch name is required for worktree")
	}

	// Verify git repository
	if err := verifyGitRepo(opts.ProjectDir); err != nil {
		return "", fmt.Errorf("not a git repository: %w", err)
	}

	projectAbs, err := filepath.Abs(opts.ProjectDir)
	if err != nil {
		return "", err
	}

	projectName := filepath.Base(projectAbs)
	// Sanitize branch name for use as directory name
	branchSafe := strings.ReplaceAll(opts.Branch, "/", "-")
	wtPath := filepath.Join(filepath.Dir(projectAbs), fmt.Sprintf("%s-dock-%s", projectName, branchSafe))

	// Check if worktree path already exists
	if _, err := os.Stat(wtPath); err == nil {
		return "", fmt.Errorf("worktree path already exists: %s", wtPath)
	}

	// Build git worktree add command
	args := []string{"worktree", "add"}
	if opts.NewBranch {
		args = append(args, "-b", opts.Branch)
	}
	args = append(args, wtPath)
	if !opts.NewBranch {
		args = append(args, opts.Branch)
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = projectAbs
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git worktree add: %w", err)
	}

	return wtPath, nil
}

// Remove removes a git worktree and its directory.
func Remove(wtPath string) error {
	wtAbs, err := filepath.Abs(wtPath)
	if err != nil {
		return err
	}

	// Find the main git repo (parent of the worktree directory)
	// git worktree remove must be run from the main repository
	mainRepo, err := findMainRepo(wtAbs)
	if err != nil {
		// Fallback: just remove the directory
		return os.RemoveAll(wtAbs)
	}

	cmd := exec.Command("git", "worktree", "remove", "--force", wtAbs)
	cmd.Dir = mainRepo
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// List returns all worktrees for a given project directory.
func List(projectDir string) ([]string, error) {
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = projectDir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git worktree list: %w", err)
	}

	var paths []string
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "worktree ") {
			paths = append(paths, strings.TrimPrefix(line, "worktree "))
		}
	}
	return paths, nil
}

func verifyGitRepo(dir string) error {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = dir
	return cmd.Run()
}

func findMainRepo(wtPath string) (string, error) {
	// git rev-parse --git-common-dir gives us the common git dir
	cmd := exec.Command("git", "rev-parse", "--git-common-dir")
	cmd.Dir = wtPath
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	commonDir := strings.TrimSpace(string(out))
	// commonDir is typically <main-repo>/.git or an absolute path
	if filepath.IsAbs(commonDir) {
		return filepath.Dir(commonDir), nil
	}
	// Relative to wtPath
	abs := filepath.Join(wtPath, commonDir)
	return filepath.Dir(abs), nil
}
