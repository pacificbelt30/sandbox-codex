package cmd

import (
	"fmt"
	"github.com/pacificbelt30/codex-dock/internal/kataguard"
	"os"
	"os/exec"

	"github.com/pacificbelt30/codex-dock/internal/sandbox"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func applyRuntimeDefaults(cmd *cobra.Command) {
	for _, setting := range []struct {
		flag, key, env string
		target         *string
		fallback       string
	}{
		{"runtime", "run.runtime", "CODEX_DOCK_RUNTIME", &runOpts.Runtime, sandbox.RuntimeKata},
		{"kata-runtime", "run.kata_runtime", "CODEX_DOCK_KATA_RUNTIME", &runOpts.KataRuntime, sandbox.DefaultKataRuntime},
	} {
		if cmd.Flags().Changed(setting.flag) {
			continue
		}
		*setting.target = setting.fallback
		if viper.IsSet(setting.key) {
			*setting.target = viper.GetString(setting.key)
		}
		if v, ok := os.LookupEnv(setting.env); ok {
			*setting.target = v
		}
	}
}

func init() {
	check := &cobra.Command{
		Use: "runtime-check", Short: "Check the containerd node and pinned Dragonball profile",
		RunE: func(cmd *cobra.Command, args []string) error {
			applyRuntimeDefaults(cmd)
			m, err := sandbox.NewManager(managementConfig())
			if err != nil {
				return err
			}
			defer func() { _ = m.Close() }()
			if err := m.CheckRuntime(cmd.Context(), runOpts); err != nil {
				return err
			}
			_, registration, err := sandbox.ResolveRuntime(runOpts.Runtime, runOpts.KataRuntime)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Runtime/node configuration verified: %s. Each worker is verified before its command is released.\n", registration)
			return nil
		},
	}
	check.Flags().StringVar(&runOpts.Runtime, "runtime", sandbox.RuntimeKata, "Worker isolation: kata or runc")
	check.Flags().StringVar(&runOpts.KataRuntime, "kata-runtime", sandbox.DefaultKataRuntime, "Kata v2 runtime registration")
	rootCmd.AddCommand(check)
	var kataPath, daemonPath, containerd, launcher, output, runtime, shim string
	seal := &cobra.Command{Use: "runtime-lock", Short: "Pin already-verified local Kata assets in a new manifest", RunE: func(cmd *cobra.Command, args []string) error {
		if containerd == "" {
			var err error
			containerd, err = exec.LookPath("containerd")
			if err != nil {
				return err
			}
		}
		return kataguard.Seal(kataPath, daemonPath, containerd, launcher, runtime, output, shim)
	}}
	seal.Flags().StringVar(&kataPath, "kata-config", "/etc/codex-dock/kata.toml", "Installed Kata profile")
	seal.Flags().StringVar(&daemonPath, "daemon-config", "/etc/codex-dock/containerd.toml", "Dedicated containerd config")
	seal.Flags().StringVar(&containerd, "containerd", "", "Trusted containerd executable (default: PATH)")
	seal.Flags().StringVar(&launcher, "launcher", "/opt/codex-dock/worker-guard", "Static trusted worker bootstrap")
	seal.Flags().StringVar(&runtime, "kata-runtime", sandbox.DefaultKataRuntime, "Kata registration")
	seal.Flags().StringVar(&output, "output", "/etc/codex-dock/kata-assets.json", "New manifest path; never overwritten")
	seal.Flags().StringVar(&shim, "runtime-shim", "/opt/kata/runtime-rs/bin/containerd-shim-kata-v2", "Pinned embedded Dragonball runtime")
	rootCmd.AddCommand(seal)
}
