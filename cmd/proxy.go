package cmd

import (
	"fmt"

	"github.com/pacificbelt30/codex-dock/internal/authproxy"
	"github.com/spf13/cobra"
)

var (
	proxyListenAddr  string
	proxyAdminSecret string
)

var proxyCmd = &cobra.Command{
	Use:    "proxy",
	Short:  "Manage auth proxy service",
	Hidden: true,
}

var proxyServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run auth proxy server",
	RunE: func(cmd *cobra.Command, args []string) error {
		p, err := authproxy.NewProxy(authproxy.Config{
			TokenTTL:    3600,
			Verbose:     verbose,
			ListenAddr:  proxyListenAddr,
			AdminSecret: proxyAdminSecret,
		})
		if err != nil {
			return err
		}
		if err := p.Start(); err != nil {
			return err
		}
		fmt.Printf("auth proxy running at %s\n", p.Endpoint())
		select {}
	},
}

func init() {
	rootCmd.AddCommand(proxyCmd)
	proxyCmd.AddCommand(proxyServeCmd)

	proxyServeCmd.Flags().StringVar(&proxyListenAddr, "listen", "0.0.0.0:18080", "listen address")
	proxyServeCmd.Flags().StringVar(&proxyAdminSecret, "admin-secret", "", "admin secret for /admin/* endpoints")
}
