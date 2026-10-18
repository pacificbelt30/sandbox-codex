package cmd

import (
	"errors"
	"fmt"

	"github.com/pacificbelt30/codex-dock/internal/sandbox"
	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start NAME|ID...",
	Short: "Start one or more stopped worker containers",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := sandbox.NewManager(managementConfig())
		if err != nil {
			return err
		}
		defer func() { _ = mgr.Close() }()
		var failures error
		for _, name := range args {
			if err := mgr.StartByName(name); err != nil {
				failures = errors.Join(failures, fmt.Errorf("starting %s: %w", name, err))
			} else if _, err := fmt.Fprintln(cmd.OutOrStdout(), name); err != nil {
				return err
			}
		}
		return failures
	},
}

func init() { rootCmd.AddCommand(startCmd) }
