package cmd

import (
	"context"
	"github.com/pacificbelt30/codex-dock/internal/engine"
	"github.com/spf13/cobra"
	"time"
)

func init() {
	var agent string
	var asJSON bool
	image := &cobra.Command{Use: "image", Short: "Inspect worker image compatibility"}
	check := &cobra.Command{Use: "check IMAGE", Short: "Check platform, tools and auth entrypoint without running image code", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(cmd.Context(), 40*time.Second)
		defer cancel()
		report, err := engine.CheckImage(ctx, args[0], agent)
		if err != nil {
			return err
		}
		if err = report.Write(cmd.OutOrStdout(), asJSON); err != nil {
			return err
		}
		return report.Err()
	}}
	check.Flags().StringVar(&agent, "agent", "shell", "Required agent executable: shell, codex or claude")
	check.Flags().BoolVar(&asJSON, "json", false, "Print structured checks")
	image.AddCommand(check)
	rootCmd.AddCommand(image)
}
