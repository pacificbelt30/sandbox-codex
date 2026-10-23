package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/pacificbelt30/codex-dock/internal/buildinfo"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile string
	verbose bool
	debug   bool
)

var rootCmd = &cobra.Command{
	Use:           "oaiso",
	Version:       buildinfo.Version,
	Short:         "御AgentISOlation (Oaiso) — isolated execution for coding agents",
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		if cache, ok := cmd.Context().Value(profileCacheKey{}).(*profileCache); ok {
			cache.command = cmd
		}
		return initConfig()
	},
	Long: `御AgentISOlation (Oaiso) runs Codex CLI and Claude Code in Kata VM-isolated workers
managed through containerd and Dragonball, with auth proxy, network separation and parallel workers.
Kata requires a Linux/KVM execution host. Runc compatibility is explicit.`,
}

func Execute() {
	cache := newProfileCache(rootCmd)
	err := rootCmd.ExecuteContext(context.WithValue(context.Background(), profileCacheKey{}, cache))
	cache.close()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		var exitErr interface{ ExitCode() int }
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().Int("passphrase-fd", -1, "Read an encryption passphrase from an explicitly supplied descriptor >= 3")
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ~/.config/codex-dock/config.toml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "Enable debug output")
	rootCmd.PersistentFlags().StringVar(&authProfileName, "auth-profile", "", "Independent credential profile; never falls back to host credentials")
	rootCmd.PersistentFlags().StringVar(&authProfileDir, "auth-profile-dir", "", "Product profile root (default: ~/.config/codex-dock/profiles)")

	_ = viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose"))
	_ = viper.BindPFlag("debug", rootCmd.PersistentFlags().Lookup("debug"))
}

func initConfig() error {
	// Built-in defaults so commands that read settings via viper (e.g. the
	// proxy image used by `proxy build`/`proxy run`) work even when no config
	// file or environment override is present.
	viper.SetDefault("default_image", "codex-dock:latest")
	viper.SetDefault("proxy_image", "codex-dock-proxy:latest")
	viper.SetDefault("default_token_ttl", 3600)
	viper.SetDefault("network_name", "dock-net-proxy")
	viper.SetDefault("proxy.forward_default_deny", true)

	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		if err == nil {
			viper.AddConfigPath(home + "/.config/codex-dock")
		}
		viper.SetConfigName("config")
		viper.SetConfigType("toml")
	}

	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		var missing viper.ConfigFileNotFoundError
		if cfgFile != "" || !errors.As(err, &missing) {
			return fmt.Errorf("reading configuration: %w", err)
		}
	} else if debug {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}
	if _, err := strconv.ParseBool(fmt.Sprint(viper.Get("proxy.forward_default_deny"))); err != nil {
		return fmt.Errorf("proxy.forward_default_deny must be a boolean: %w", err)
	}
	return nil
}
