package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	dockerdefaults "github.com/pacificbelt30/codex-dock/docker"
	"github.com/pacificbelt30/codex-dock/internal/authproxy"
	"github.com/spf13/cobra"
)

var (
	proxyListenAddr  string
	proxyAdminSecret string
	proxyImage       string
	proxyDockerfile  string
	proxyContainer   string
	proxyNetwork     string
)

var proxyCmd = &cobra.Command{
	Use:   "proxy",
	Short: "Manage auth proxy service",
}

var proxyBuildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build auth proxy Docker image",
	RunE: func(cmd *cobra.Command, args []string) error {
		dockerfile, buildCtx, err := resolveProxyDockerfile(proxyDockerfile)
		if err != nil {
			return err
		}
		return executeBuild(proxyImage, dockerfile, buildCtx)
	},
}

var proxyRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run auth proxy container",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := exec.Command("docker", "network", "inspect", proxyNetwork).Run(); err != nil {
			fmt.Printf("Network %s not found. Creating...\n", proxyNetwork)
			create := exec.Command("docker", "network", "create", proxyNetwork)
			create.Stdout = os.Stdout
			create.Stderr = os.Stderr
			if err := create.Run(); err != nil {
				return fmt.Errorf("create network %s: %w", proxyNetwork, err)
			}
		}

		rm := exec.Command("docker", "rm", "-f", proxyContainer)
		rm.Stdout = os.Stdout
		rm.Stderr = os.Stderr
		_ = rm.Run()

		run := exec.Command(
			"docker", "run", "-d",
			"--name", proxyContainer,
			"--network", proxyNetwork,
			"-p", "127.0.0.1:18080:18080",
			"-e", "CODEX_PROXY_ADMIN_SECRET="+proxyAdminSecret,
			proxyImage,
		)
		run.Stdout = os.Stdout
		run.Stderr = os.Stderr
		if err := run.Run(); err != nil {
			return fmt.Errorf("docker run failed: %w", err)
		}
		fmt.Printf("Auth proxy container %s is running on http://127.0.0.1:18080\n", proxyContainer)
		return nil
	},
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
	proxyCmd.AddCommand(proxyBuildCmd)
	proxyCmd.AddCommand(proxyRunCmd)
	proxyCmd.AddCommand(proxyServeCmd)

	proxyBuildCmd.Flags().StringVarP(&proxyImage, "tag", "t", "codex-dock-auth-proxy:latest", "Image tag")
	proxyBuildCmd.Flags().StringVarP(&proxyDockerfile, "dockerfile", "f", "", "Path to auth proxy Dockerfile")

	proxyRunCmd.Flags().StringVarP(&proxyImage, "image", "i", "codex-dock-auth-proxy:latest", "Auth proxy image")
	proxyRunCmd.Flags().StringVarP(&proxyContainer, "name", "n", "codex-auth-proxy", "Container name")
	proxyRunCmd.Flags().StringVar(&proxyNetwork, "network", "dock-net", "Docker network")
	proxyRunCmd.Flags().StringVar(&proxyAdminSecret, "admin-secret", "", "admin secret for /admin/* endpoints")

	proxyServeCmd.Flags().StringVar(&proxyListenAddr, "listen", "0.0.0.0:18080", "listen address")
	proxyServeCmd.Flags().StringVar(&proxyAdminSecret, "admin-secret", "", "admin secret for /admin/* endpoints")
}

func resolveProxyDockerfile(flagValue string) (string, string, error) {
	if flagValue != "" {
		return flagValue, ".", nil
	}

	for _, p := range []string{"docker/auth-proxy.Dockerfile", "auth-proxy.Dockerfile"} {
		if _, err := os.Stat(p); err == nil {
			return p, ".", nil
		}
	}

	configDir, err := defaultConfigDir()
	if err != nil {
		return "", "", fmt.Errorf("auth proxy Dockerfile not found; use -f to specify path")
	}
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return "", "", err
	}

	dfPath := filepath.Join(configDir, "auth-proxy.Dockerfile")
	if _, err := os.Stat(dfPath); os.IsNotExist(err) {
		if err := os.WriteFile(dfPath, dockerdefaults.AuthProxyDockerfile, 0644); err != nil {
			return "", "", err
		}
	}

	return dfPath, ".", nil
}
