package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

const updateModulePath = "github.com/pacificbelt30/codex-dock@latest"

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update codex-dock to the latest version",
	Long: `update runs "go install github.com/pacificbelt30/codex-dock@latest",
fetching and building the latest release into $(go env GOPATH)/bin.

This only updates installations done via "go install" - it does not touch
binaries installed with "make install" or a prebuilt release.`,
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

	fmt.Printf("Updating codex-dock via go install %s...\n", updateModulePath)
	c := exec.CommandContext(ctx, goBin, "install", updateModulePath)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("go install failed: %w", err)
	}
	fmt.Println("codex-dock updated successfully.")
	return nil
}
