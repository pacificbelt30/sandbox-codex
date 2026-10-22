package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

const updateModulePath = "github.com/pacificbelt30/codex-dock/cmd/oaiso@latest"

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update oaiso to the latest version",
	Long: `update runs "go install github.com/pacificbelt30/codex-dock/cmd/oaiso@latest",
fetching and building oaiso into GOBIN or $(go env GOPATH)/bin.

Only oaiso is installed; no legacy command aliases are created.
Requires a published release containing cmd/oaiso.
It does not update binaries installed with "make install" or a node bundle.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return executeUpdate(cmd.Context())
	},
}

func init() {
	rootCmd.AddCommand(updateCmd)
}

// executeUpdate installs the canonical oaiso entry point directly.
func executeUpdate(ctx context.Context) error {
	goBin, err := exec.LookPath("go")
	if err != nil {
		return fmt.Errorf("go toolchain not found in PATH: %w", err)
	}

	fmt.Printf("Updating oaiso via go install %s...\n", updateModulePath)
	c := exec.CommandContext(ctx, goBin, "install", updateModulePath)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("go install failed: %w", err)
	}
	fmt.Println("oaiso updated successfully.")
	return nil
}
