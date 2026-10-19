package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

const updateModulePath = "github.com/pacificbelt30/codex-dock@latest"

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update cxd to the latest version",
	Long: `update runs "go install github.com/pacificbelt30/codex-dock@latest",
fetching and building the latest release into $(go env GOPATH)/bin.

The module installs as codex-dock. This command also maintains the cxd
command in the same Go bin directory. It does not update binaries installed
with "make install" or a node bundle.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return executeUpdate(cmd.Context())
	},
}

func init() {
	rootCmd.AddCommand(updateCmd)
}

// executeUpdate runs "go install" against the latest codex-dock module.
func executeUpdate(ctx context.Context) error {
	goBin, err := exec.LookPath("go")
	if err != nil {
		return fmt.Errorf("go toolchain not found in PATH: %w", err)
	}

	fmt.Printf("Updating cxd via go install %s...\n", updateModulePath)
	c := exec.CommandContext(ctx, goBin, "install", updateModulePath)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("go install failed: %w", err)
	}
	if err := ensureGoInstalledAlias(ctx, goBin); err != nil {
		return err
	}
	fmt.Println("cxd updated successfully.")
	return nil
}

func ensureGoInstalledAlias(ctx context.Context, goBin string) error {
	command := exec.CommandContext(ctx, goBin, "env", "GOBIN", "GOPATH")
	output, err := command.Output()
	if err != nil {
		return fmt.Errorf("locating Go bin directory: %w", err)
	}
	values := strings.Split(strings.TrimSuffix(string(output), "\n"), "\n")
	if len(values) < 2 {
		return fmt.Errorf("go env returned no GOPATH")
	}
	binDir := strings.TrimSuffix(values[0], "\r")
	if binDir == "" {
		paths := filepath.SplitList(strings.TrimSuffix(values[1], "\r"))
		if len(paths) == 0 || paths[0] == "" {
			return fmt.Errorf("go env returned no GOPATH")
		}
		binDir = filepath.Join(paths[0], "bin")
	}
	name, aliasName := "codex-dock", "cxd"
	if runtime.GOOS == "windows" {
		name += ".exe"
		aliasName += ".exe"
	}
	installed := filepath.Join(binDir, name)
	alias := filepath.Join(binDir, aliasName)
	if _, err := os.Stat(installed); err != nil {
		return fmt.Errorf("go install output missing: %w", err)
	}
	if info, err := os.Lstat(alias); err == nil {
		if info.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("%s already exists; leave it untouched and update it with its original installer", alias)
		}
		target, err := os.Readlink(alias)
		if err != nil || target != name {
			return fmt.Errorf("%s points elsewhere; leave it untouched", alias)
		}
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Symlink(name, alias); err != nil {
		return fmt.Errorf("creating cxd command in %s: %w", binDir, err)
	}
	return nil
}
