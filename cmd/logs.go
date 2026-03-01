package cmd

import (
	"fmt"

	"github.com/pacificbelt30/codex-dock/internal/sandbox"
	"github.com/spf13/cobra"
)

var (
	logsTail   int
	logsFollow bool
)

var logsCmd = &cobra.Command{
	Use:   "logs NAME|ID",
	Short: "Fetch container logs",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := sandbox.NewManager(sandbox.ManagerConfig{Verbose: verbose})
		if err != nil {
			return err
		}

		opts := sandbox.LogOptions{
			Name:   args[0],
			Tail:   logsTail,
			Follow: logsFollow,
		}

		if err := mgr.Logs(opts); err != nil {
			return fmt.Errorf("fetching logs: %w", err)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(logsCmd)
	logsCmd.Flags().IntVarP(&logsTail, "tail", "n", 100, "Number of lines to show from end")
	logsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", false, "Follow log output")
}
