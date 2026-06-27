package cmd

import (
	"fmt"

	"github.com/pacificbelt30/codex-dock/internal/network"
	"github.com/pacificbelt30/codex-dock/internal/sandbox"
	"github.com/spf13/cobra"
)

var rmForce bool

var rmCmd = &cobra.Command{
	Use:   "rm [NAME|ID...]",
	Short: "Remove stopped worker containers",
	RunE: func(cmd *cobra.Command, args []string) error {
		// A network manager is required so Remove also tears down each worker's
		// per-worker Internal network (otherwise the networks accumulate).
		netMgr, err := network.NewManager()
		if err != nil {
			return fmt.Errorf("creating network manager: %w", err)
		}
		mgr, err := sandbox.NewManager(sandbox.ManagerConfig{Network: netMgr, Verbose: verbose})
		if err != nil {
			return err
		}

		if len(args) == 0 {
			// Remove all stopped containers
			workers, err := mgr.List(true)
			if err != nil {
				return err
			}
			for _, w := range workers {
				if w.Status == "running" && !rmForce {
					fmt.Printf("Skipping running container %s (use --force to remove)\n", w.Name)
					continue
				}
				fmt.Printf("Removing %s...\n", w.Name)
				if err := mgr.Remove(w.ID, rmForce); err != nil {
					fmt.Printf("  error: %v\n", err)
				}
			}
			return nil
		}

		for _, name := range args {
			fmt.Printf("Removing %s...\n", name)
			if err := mgr.RemoveByName(name, rmForce); err != nil {
				fmt.Printf("  error: %v\n", err)
			}
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(rmCmd)
	rmCmd.Flags().BoolVarP(&rmForce, "force", "f", false, "Force removal of running containers")
}
