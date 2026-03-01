package worktree

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initGitRepo creates a temporary git repository with an initial commit.
func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	cmds := [][]string{
		{"git", "init", "-b", "main"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "config", "commit.gpgsign", "false"},
		{"git", "commit", "--allow-empty", "-m", "init"},
	}
	for _, args := range cmds {
		c := exec.Command(args[0], args[1:]...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("cmd %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

func TestVerifyGitRepo_Valid(t *testing.T) {
	dir := initGitRepo(t)
	if err := verifyGitRepo(dir); err != nil {
		t.Errorf("verifyGitRepo on valid repo: %v", err)
	}
}

func TestVerifyGitRepo_Invalid(t *testing.T) {
	dir := t.TempDir()
	if err := verifyGitRepo(dir); err == nil {
		t.Error("expected error for non-git directory")
	}
}

func TestCreate_NewBranch(t *testing.T) {
	projectDir := initGitRepo(t)

	opts := CreateOptions{
		ProjectDir: projectDir,
		Branch:     "feature-test",
		NewBranch:  true,
	}

	wtPath, err := Create(opts)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer Remove(wtPath)

	if _, err := os.Stat(wtPath); err != nil {
		t.Errorf("worktree directory does not exist: %v", err)
	}

	projectName := filepath.Base(projectDir)
	expectedSuffix := projectName + "-dock-feature-test"
	if !strings.HasSuffix(wtPath, expectedSuffix) {
		t.Errorf("worktree path %q does not end with %q", wtPath, expectedSuffix)
	}
}

func TestCreate_ExistingBranch(t *testing.T) {
	projectDir := initGitRepo(t)

	// Create the branch first
	c := exec.Command("git", "branch", "existing-branch")
	c.Dir = projectDir
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("git branch: %v\n%s", err, out)
	}

	opts := CreateOptions{
		ProjectDir: projectDir,
		Branch:     "existing-branch",
		NewBranch:  false,
	}

	wtPath, err := Create(opts)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer Remove(wtPath)

	if _, err := os.Stat(wtPath); err != nil {
		t.Errorf("worktree directory does not exist: %v", err)
	}
}

func TestCreate_NoBranch(t *testing.T) {
	dir := initGitRepo(t)
	_, err := Create(CreateOptions{ProjectDir: dir})
	if err == nil {
		t.Error("expected error when no branch specified")
	}
}

func TestCreate_NotGitRepo(t *testing.T) {
	dir := t.TempDir()
	_, err := Create(CreateOptions{ProjectDir: dir, Branch: "test", NewBranch: true})
	if err == nil {
		t.Error("expected error for non-git directory")
	}
}

func TestCreate_AlreadyExists(t *testing.T) {
	projectDir := initGitRepo(t)

	opts := CreateOptions{
		ProjectDir: projectDir,
		Branch:     "duplicate-branch",
		NewBranch:  true,
	}

	// First creation succeeds
	wtPath, err := Create(opts)
	if err != nil {
		t.Fatalf("first Create: %v", err)
	}
	defer Remove(wtPath)

	// Second creation with same branch name and path should fail
	_, err = Create(opts)
	if err == nil {
		t.Error("expected error when worktree path already exists")
	}
}

func TestCreate_SlashInBranch(t *testing.T) {
	projectDir := initGitRepo(t)

	opts := CreateOptions{
		ProjectDir: projectDir,
		Branch:     "feature/auth-module",
		NewBranch:  true,
	}

	wtPath, err := Create(opts)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer Remove(wtPath)

	// Slash should be replaced by dash in path
	if strings.Contains(filepath.Base(wtPath), "/") {
		t.Errorf("worktree path %q contains slash", wtPath)
	}
	if !strings.Contains(filepath.Base(wtPath), "feature-auth-module") {
		t.Errorf("worktree path %q does not contain 'feature-auth-module'", wtPath)
	}
}

func TestRemove(t *testing.T) {
	projectDir := initGitRepo(t)

	opts := CreateOptions{
		ProjectDir: projectDir,
		Branch:     "remove-me",
		NewBranch:  true,
	}

	wtPath, err := Create(opts)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := Remove(wtPath); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Error("worktree directory still exists after Remove")
	}
}

func TestList(t *testing.T) {
	projectDir := initGitRepo(t)

	// Should have at least the main worktree
	paths, err := List(projectDir)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(paths) == 0 {
		t.Error("expected at least 1 worktree (the main repo)")
	}

	// Add a worktree
	opts := CreateOptions{
		ProjectDir: projectDir,
		Branch:     "list-branch",
		NewBranch:  true,
	}
	wtPath, err := Create(opts)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer Remove(wtPath)

	paths, err = List(projectDir)
	if err != nil {
		t.Fatalf("List after add: %v", err)
	}
	if len(paths) < 2 {
		t.Errorf("expected at least 2 worktrees, got %d", len(paths))
	}

	found := false
	for _, p := range paths {
		if p == wtPath {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("worktree %q not found in list: %v", wtPath, paths)
	}
}
