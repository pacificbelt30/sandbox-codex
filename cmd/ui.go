package cmd

import (
	"github.com/pacificbelt30/codex-dock/internal/ui"
	"github.com/spf13/cobra"
)

var uiCmd = &cobra.Command{
	Use:   "ui",
	Short: "Launch dock-ui TUI container manager",
	RunE: func(cmd *cobra.Command, args []string) error {
		return ui.Run()
	},
}

func init() {
	rootCmd.AddCommand(uiCmd)
}
