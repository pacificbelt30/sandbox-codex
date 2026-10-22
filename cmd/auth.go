package cmd

import (
	"fmt"
	"os"

	"github.com/pacificbelt30/codex-dock/internal/authproxy"
	"github.com/spf13/cobra"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage authentication credentials",
}

var authShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current auth configuration (without secrets)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if profile := selectedAuthProfile(); profile != "" {
			return showAuthProfile(cmd, profile)
		}
		info, err := authproxy.GetAuthInfo()
		if err != nil {
			return fmt.Errorf("getting auth info: %w", err)
		}
		fmt.Printf("Auth source: %s\n", info.Source)
		fmt.Printf("Key configured: %v\n", info.KeyConfigured)
		return nil
	},
}

var authSetCmd = &cobra.Command{
	Use:   "set",
	Short: "Set API key from environment variable OPENAI_API_KEY",
	RunE: func(cmd *cobra.Command, args []string) error {
		if selectedAuthProfile() != "" {
			return fmt.Errorf("use auth profile set NAME for independent profiles; auth set is the legacy environment importer")
		}
		key := os.Getenv("OPENAI_API_KEY")
		if key == "" {
			return fmt.Errorf("OPENAI_API_KEY environment variable not set")
		}
		if err := authproxy.SaveAPIKey(key); err != nil {
			return fmt.Errorf("saving API key: %w", err)
		}
		fmt.Println("API key saved to codex-dock config.")
		return nil
	},
}

var authRotateCmd = &cobra.Command{
	Use:   "rotate",
	Short: "Rotate all active tokens issued by Auth Proxy",
	RunE: func(cmd *cobra.Command, args []string) error {
		if selectedAuthProfile() != "" {
			return fmt.Errorf("stop profile workers or use the proxy admin revocation API; the legacy rotate marker does not rotate independent profiles")
		}
		fmt.Println("Rotating tokens... (restart auth proxy to apply)")
		return authproxy.RotateTokens()
	},
}

func init() {
	rootCmd.AddCommand(authCmd)
	authCmd.AddCommand(authShowCmd)
	authCmd.AddCommand(authSetCmd)
	authCmd.AddCommand(authRotateCmd)
	authCmd.AddCommand(newAuthProfileCommand())
}
