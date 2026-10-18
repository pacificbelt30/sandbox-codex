package cmd

import (
	"strings"

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

func codexOptionPresent(args []string, names ...string) bool {
	for _, arg := range args {
		if arg == "--" {
			break
		}
		for _, name := range names {
			if arg == name || strings.HasPrefix(arg, name+"=") || len(name) == 2 && name[0] == '-' && name[1] != '-' && strings.HasPrefix(arg, name) {
				return true
			}
		}
	}
	return false
}

// Codex -c/--config values are native CLI overrides, so they take precedence
// over defaults injected from codex-dock's global or project configuration.
func codexConfigOverridePresent(args []string, key string) bool {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			break
		}
		value := ""
		switch {
		case arg == "-c" || arg == "--config":
			if i+1 < len(args) {
				if args[i+1] == "--" {
					return false
				}
				i++
				value = args[i]
			}
		case strings.HasPrefix(arg, "--config="):
			value = strings.TrimPrefix(arg, "--config=")
		case strings.HasPrefix(arg, "-c") && len(arg) > 2:
			value = strings.TrimPrefix(arg, "-c")
		}
		name, _, hasValue := strings.Cut(value, "=")
		if hasValue && strings.TrimSpace(name) == key {
			return true
		}
	}
	return false
}
