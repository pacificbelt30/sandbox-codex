package cmd

import (
	"fmt"

	"github.com/pacificbelt30/codex-dock/internal/sandbox"
	"github.com/spf13/cobra"
)

var stopAll bool
var stopTimeout int

var stopCmd = &cobra.Command{
	Use:   "stop [NAME|ID...]",
	Short: "Stop one or more worker containers",
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := sandbox.NewManager(sandbox.ManagerConfig{Verbose: verbose})
		if err != nil {
			return err
		}

		if stopAll {
			workers, err := mgr.List(false)
			if err != nil {
				return err
			}
			for _, w := range workers {
				fmt.Printf("Stopping %s...\n", w.Name)
				if err := mgr.Stop(w.ID, stopTimeout); err != nil {
					fmt.Printf("  error: %v\n", err)
				}
			}
			return nil
		}

		if len(args) == 0 {
			return fmt.Errorf("specify container names/IDs or use --all")
		}

		for _, name := range args {
			fmt.Printf("Stopping %s...\n", name)
			if err := mgr.StopByName(name, stopTimeout); err != nil {
				fmt.Printf("  error: %v\n", err)
			}
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(stopCmd)
	stopCmd.Flags().BoolVarP(&stopAll, "all", "a", false, "Stop all running containers")
	stopCmd.Flags().IntVar(&stopTimeout, "timeout", 10, "Timeout in seconds before force kill")
}
