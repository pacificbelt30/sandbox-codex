package cmd

import (
	"github.com/pacificbelt30/codex-dock/internal/sandbox"
	"github.com/spf13/cobra"
)

// codexCmd forwards Codex's own flags and prompt without interpreting them as
// codex-dock flags. Runtime and image defaults still come from config.toml.
var codexCmd = &cobra.Command{
	Use:                "codex [CODEX_ARGS...]",
	Short:              "Launch Codex directly in a containerd/Kata worker",
	DisableFlagParsing: true,
	Args:               cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		agentFlag = string(sandbox.AgentCodex)
		runOpts.DirectAgent = true
		runOpts.AgentArgs = append([]string(nil), args...)
		return runWorker(cmd, nil)
	},
}

func init() { rootCmd.AddCommand(codexCmd) }
