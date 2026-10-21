package cmd

import (
	"github.com/pacificbelt30/codex-dock/internal/sandbox"
	"github.com/spf13/cobra"
)

var claudeCmd = &cobra.Command{
	Use: "claude [CLAUDE_ARGS...]", Short: "Launch Claude Code directly in a containerd/Kata worker",
	DisableFlagParsing: true, Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		agentFlag = string(sandbox.AgentClaude)
		runOpts.DirectAgent = true
		runOpts.AgentArgs = append([]string(nil), args...)
		return runWorker(cmd, nil)
	},
}

func init() { rootCmd.AddCommand(claudeCmd) }
