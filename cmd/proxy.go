package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	dockerdefaults "github.com/pacificbelt30/codex-dock/docker"
	"github.com/pacificbelt30/codex-dock/internal/authproxy"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const defaultProxyContainerName = "codex-dock-proxy"

var (
	proxyListenAddr  string
	proxyAdminSecret string

	proxyBuildTag        string
	proxyBuildDockerfile string

	proxyRunName string
	proxyRunPort int

	proxyStopName string

	proxyRmName  string
	proxyRmForce bool
)

var proxyCmd = &cobra.Command{
	Use:   "proxy",
	Short: "Manage auth proxy service and container",
}

// proxy serve ---------------------------------------------------------------

var proxyServeCmd = &cobra.Command{
	Use:    "serve",
	Short:  "Run auth proxy server (in-process)",
	Hidden: true,
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

// proxy build ---------------------------------------------------------------

var proxyBuildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build the auth proxy Docker image",
	RunE: func(cmd *cobra.Command, args []string) error {
		dockerfile, buildCtx, err := resolveProxyDockerfile(proxyBuildDockerfile)
		if err != nil {
			return err
		}
		tag := viper.GetString("proxy_image")
		if proxyBuildTag != "" {
			tag = proxyBuildTag
		}
		return executeProxyBuild(tag, dockerfile, buildCtx)
	},
}

// proxy run -----------------------------------------------------------------

var proxyRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the auth proxy as a Docker container",
	RunE: func(cmd *cobra.Command, args []string) error {
		image := viper.GetString("proxy_image")
		listenAddr := fmt.Sprintf("0.0.0.0:%d", proxyRunPort)
		portMapping := fmt.Sprintf("%d:%d", proxyRunPort, proxyRunPort)

		fmt.Printf("Starting proxy container %q (image: %s, port: %s)...\n", proxyRunName, image, portMapping)
		c := exec.Command("docker", "run", "-d",
			"--name", proxyRunName,
			"-p", portMapping,
			image,
			"proxy", "serve",
			"--listen", listenAddr,
		)
		if proxyAdminSecret != "" {
			c.Args = append(c.Args, "--admin-secret", proxyAdminSecret)
		}
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			return fmt.Errorf("docker run failed: %w", err)
		}
		fmt.Printf("Proxy container %q started.\n", proxyRunName)
		return nil
	},
}

// proxy stop ----------------------------------------------------------------

var proxyStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the auth proxy container",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("Stopping proxy container %q...\n", proxyStopName)
		c := exec.Command("docker", "stop", proxyStopName)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			return fmt.Errorf("docker stop failed: %w", err)
		}
		fmt.Printf("Proxy container %q stopped.\n", proxyStopName)
		return nil
	},
}

// proxy rm ------------------------------------------------------------------

var proxyRmCmd = &cobra.Command{
	Use:   "rm",
	Short: "Remove the auth proxy container",
	RunE: func(cmd *cobra.Command, args []string) error {
		dockerArgs := []string{"rm"}
		if proxyRmForce {
			dockerArgs = append(dockerArgs, "--force")
		}
		dockerArgs = append(dockerArgs, proxyRmName)

		fmt.Printf("Removing proxy container %q...\n", proxyRmName)
		c := exec.Command("docker", dockerArgs...)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			return fmt.Errorf("docker rm failed: %w", err)
		}
		fmt.Printf("Proxy container %q removed.\n", proxyRmName)
		return nil
	},
}

// ---------------------------------------------------------------------------

func init() {
	rootCmd.AddCommand(proxyCmd)
	proxyCmd.AddCommand(proxyServeCmd)
	proxyCmd.AddCommand(proxyBuildCmd)
	proxyCmd.AddCommand(proxyRunCmd)
	proxyCmd.AddCommand(proxyStopCmd)
	proxyCmd.AddCommand(proxyRmCmd)

	// serve flags
	proxyServeCmd.Flags().StringVar(&proxyListenAddr, "listen", "0.0.0.0:18080", "listen address")
	proxyServeCmd.Flags().StringVar(&proxyAdminSecret, "admin-secret", "", "admin secret for /admin/* endpoints")

	// build flags
	proxyBuildCmd.Flags().StringVarP(&proxyBuildTag, "tag", "t", "", "Image tag (default: proxy_image from config)")
	proxyBuildCmd.Flags().StringVarP(&proxyBuildDockerfile, "dockerfile", "f", "", "Path to auth-proxy.Dockerfile")

	// run flags
	proxyRunCmd.Flags().StringVar(&proxyRunName, "name", defaultProxyContainerName, "Container name")
	proxyRunCmd.Flags().IntVarP(&proxyRunPort, "port", "p", 18080, "Host port to expose the proxy on")
	proxyRunCmd.Flags().StringVar(&proxyAdminSecret, "admin-secret", "", "admin secret for /admin/* endpoints")

	// stop flags
	proxyStopCmd.Flags().StringVar(&proxyStopName, "name", defaultProxyContainerName, "Container name")

	// rm flags
	proxyRmCmd.Flags().StringVar(&proxyRmName, "name", defaultProxyContainerName, "Container name")
	proxyRmCmd.Flags().BoolVarP(&proxyRmForce, "force", "f", false, "Force remove a running container")
}

// resolveProxyDockerfile returns the auth-proxy Dockerfile path and build context to use.
// Priority: explicit -f flag > auth-proxy.Dockerfile / docker/auth-proxy.Dockerfile in CWD > config-dir default.
// The build context is always "." (CWD) because the proxy image compiles Go source from the repo root.
func resolveProxyDockerfile(flagValue string) (string, string, error) {
	if flagValue != "" {
		return flagValue, ".", nil
	}

	// Check well-known locations relative to the current directory.
	for _, p := range []string{"auth-proxy.Dockerfile", "docker/auth-proxy.Dockerfile"} {
		if _, err := os.Stat(p); err == nil {
			return p, ".", nil
		}
	}

	// Fall back to the default Dockerfile written into the config directory.
	configDir, err := defaultConfigDir()
	if err != nil {
		return "", "", fmt.Errorf("auth-proxy.Dockerfile not found; use -f to specify path")
	}
	if err := ensureProxyDockerfile(configDir); err != nil {
		return "", "", fmt.Errorf("writing proxy Dockerfile to config dir: %w", err)
	}
	return filepath.Join(configDir, "auth-proxy.Dockerfile"), ".", nil
}

// ensureProxyDockerfile writes the embedded auth-proxy.Dockerfile into dir if not already present.
func ensureProxyDockerfile(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	dfPath := filepath.Join(dir, "auth-proxy.Dockerfile")
	if _, err := os.Stat(dfPath); os.IsNotExist(err) {
		if err := os.WriteFile(dfPath, dockerdefaults.ProxyDockerfile, 0644); err != nil {
			return err
		}
	}
	return nil
}

// executeProxyBuild runs "docker build" for the auth proxy image.
func executeProxyBuild(tag, dockerfile, buildCtx string) error {
	fmt.Printf("Building proxy image %s from %s...\n", tag, dockerfile)
	c := exec.Command("docker", "build", "-t", tag, "-f", dockerfile, buildCtx)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("docker build failed: %w", err)
	}
	fmt.Printf("Proxy image %s built successfully.\n", tag)
	return nil
}
