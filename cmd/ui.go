package cmd

import (
	"github.com/pacificbelt30/codex-dock/internal/proxysettings"
	"github.com/pacificbelt30/codex-dock/internal/ui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"path/filepath"
)

var uiCmd = &cobra.Command{
	Use:   "ui",
	Short: "Launch the Oaiso TUI worker manager",
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := proxyConfigPath()
		if err != nil {
			return err
		}
		return ui.RunWithOptions(managementConfig(), path)
	},
}

func init() {
	rootCmd.AddCommand(uiCmd)
	proxyCmd.AddCommand(&cobra.Command{Use: "configure", Short: "Edit non-secret proxy policy settings in the TUI", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		path, err := proxyConfigPath()
		if err != nil {
			return err
		}
		return ui.RunProxySettings(path)
	}})
}

func proxyConfigPath() (string, error) {
	path := viper.ConfigFileUsed()
	if cfgFile != "" {
		path = cfgFile
	}
	if path == "" {
		return proxysettings.DefaultPath()
	}
	return filepath.Abs(path)
}
