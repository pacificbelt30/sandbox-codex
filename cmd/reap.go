package cmd

import (
	"encoding/json"
	"github.com/pacificbelt30/codex-dock/internal/sandbox"
	"github.com/spf13/cobra"
	"time"
)

func init() {
	var dry, watch bool
	c := &cobra.Command{Use: "reap", Short: "Enforce opt-in worker lifecycle policies (run --watch as a supervisor)", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		m, err := sandbox.NewManager(managementConfig())
		if err != nil {
			return err
		}
		defer func() { _ = m.Close() }()
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			actions, err := m.Reap(cmd.Context(), time.Now(), dry)
			for _, a := range actions {
				if e := json.NewEncoder(cmd.OutOrStdout()).Encode(a); e != nil {
					return e
				}
			}
			if err != nil {
				return err
			}
			if !watch {
				return nil
			}
			select {
			case <-cmd.Context().Done():
				return cmd.Context().Err()
			case <-ticker.C:
			}
		}
	}}
	c.Flags().BoolVar(&dry, "dry-run", false, "Show actions without stopping or deleting workers")
	c.Flags().BoolVar(&watch, "watch", false, "Keep supervising until canceled")
	rootCmd.AddCommand(c)
}
