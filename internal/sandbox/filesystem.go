package sandbox

import (
	"fmt"
	"github.com/pacificbelt30/codex-dock/internal/workerpolicy"
	"os"
	"path/filepath"
	"strings"
)

// OCI masks/remounts protect existing paths. Refuse missing or symlinked
// targets instead of implying that a future file would inherit a restriction.
func validateFilesystemTargets(workspace string, policy workerpolicy.Filesystem) error {
	if len(policy.DenyRead)+len(policy.DenyWrite) == 0 {
		return nil
	}
	root, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		return err
	}
	for _, paths := range [][]string{policy.DenyRead, policy.DenyWrite} {
		for _, guest := range paths {
			host := filepath.Join(root, strings.TrimPrefix(guest, "/workspace/"))
			info, err := os.Lstat(host)
			if err != nil {
				return fmt.Errorf("filesystem restriction requires an existing workspace target %s: %w", guest, err)
			}
			resolved, err := filepath.EvalSymlinks(host)
			if err != nil {
				return err
			}
			relative, err := filepath.Rel(root, resolved)
			if err != nil || !filepath.IsLocal(relative) || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() && !info.IsDir() {
				return fmt.Errorf("filesystem restriction target must resolve to a regular file or directory inside /workspace: %s", guest)
			}
		}
	}
	return nil
}
