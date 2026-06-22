package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	dockerdefaults "github.com/pacificbelt30/codex-dock/docker"
	"github.com/pacificbelt30/codex-dock/internal/authproxy"
	"github.com/pacificbelt30/codex-dock/internal/network"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const defaultProxyContainerName = "codex-auth-proxy"

var (
	proxyListenAddr  string
	proxyAdminSecret string

	proxyBuildTag        string
	proxyBuildDockerfile string

	proxyRunName    string
	proxyRunPort    int
	proxyRunNetwork string

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
		return executeProxyBuild(cmd.Context(), tag, dockerfile, buildCtx)
	},
}

// proxy run -----------------------------------------------------------------

var proxyRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the auth proxy as a Docker container",
	Long: `Run the auth proxy as a detached Docker container.

Credentials are automatically bound from the host into the container:
  - OPENAI_API_KEY env var  → injected as -e OPENAI_API_KEY=<value>
  - ~/.config/codex-dock/apikey → bind-mounted read-only (API key file)
  - ~/.codex/auth.json           → bind-mounted read-only (OAuth/ChatGPT)

At least one credential source must be configured before running.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		image := viper.GetString("proxy_image")
		listenAddr := fmt.Sprintf("0.0.0.0:%d", proxyRunPort)

		if err := ensureBridgeNetwork(cmd.Context(), proxyRunNetwork, network.ProxyBridgeName); err != nil {
			return err
		}

		home, err := os.UserHomeDir()
		if err != nil {
			home = ""
		}
		storedKeyPath := filepath.Join(home, ".config", "codex-dock", "apikey")
		oauthJSONPath := filepath.Join(home, ".codex", "auth.json")
		anthropicKeyPath := filepath.Join(home, ".config", "codex-dock", "anthropic-apikey")
		claudeCredsPath := filepath.Join(home, ".claude", ".credentials.json")

		dockerArgs := buildProxyRunArgs(proxyRunArgs{
			name:             proxyRunName,
			port:             proxyRunPort,
			networkName:      proxyRunNetwork,
			image:            image,
			listenAddr:       listenAddr,
			adminSecret:      proxyAdminSecret,
			apiKeyEnv:        os.Getenv("OPENAI_API_KEY"),
			storedKeyPath:    storedKeyPath,
			oauthJSONPath:    oauthJSONPath,
			anthropicKeyEnv:  os.Getenv("ANTHROPIC_API_KEY"),
			anthropicKeyPath: anthropicKeyPath,
			claudeCredsPath:  claudeCredsPath,
		})

		portMapping := fmt.Sprintf("%d:%d", proxyRunPort, proxyRunPort)
		fmt.Printf("Starting proxy container %q (image: %s, network: %s, port: %s)...\n", proxyRunName, image, proxyRunNetwork, portMapping)
		c := exec.CommandContext(cmd.Context(), "docker", dockerArgs...)
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
		c := exec.CommandContext(cmd.Context(), "docker", "stop", proxyStopName)
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
		c := exec.CommandContext(cmd.Context(), "docker", dockerArgs...)
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
	proxyRunCmd.Flags().StringVar(&proxyRunNetwork, "network", network.ProxyNetworkName, "Docker network to attach the proxy container to")
	proxyRunCmd.Flags().StringVar(&proxyAdminSecret, "admin-secret", "", "admin secret for /admin/* endpoints")

	// stop flags
	proxyStopCmd.Flags().StringVar(&proxyStopName, "name", defaultProxyContainerName, "Container name")

	// rm flags
	proxyRmCmd.Flags().StringVar(&proxyRmName, "name", defaultProxyContainerName, "Container name")
	proxyRmCmd.Flags().BoolVarP(&proxyRmForce, "force", "f", false, "Force remove a running container")
}

// proxyRunArgs collects the inputs needed to build the "docker run" command for
// the auth proxy container.
type proxyRunArgs struct {
	name        string
	port        int
	networkName string
	image       string
	listenAddr  string
	adminSecret string

	// OpenAI / Codex credential sources.
	apiKeyEnv     string // OPENAI_API_KEY env value
	storedKeyPath string // ~/.config/codex-dock/apikey
	oauthJSONPath string // ~/.codex/auth.json

	// Anthropic / Claude Code credential sources.
	anthropicKeyEnv  string // ANTHROPIC_API_KEY env value
	anthropicKeyPath string // ~/.config/codex-dock/anthropic-apikey
	claudeCredsPath  string // ~/.claude/.credentials.json
}

// buildProxyRunArgs constructs the argument list for "docker run" to start the
// auth proxy container. Every present credential source is bound so the
// container mirrors the host's auth state for both agents:
//
//	OpenAI/Codex : OPENAI_API_KEY env, apikey file, ~/.codex/auth.json
//	Anthropic    : ANTHROPIC_API_KEY env, anthropic-apikey file, ~/.claude/.credentials.json
//
// The proxy selects the active source per provider in priority order
// (env > stored key file > OAuth credentials).
func buildProxyRunArgs(a proxyRunArgs) []string {
	portMapping := fmt.Sprintf("%d:%d", a.port, a.port)
	args := []string{"run", "-d", "--name", a.name, "--network", a.networkName, "-p", portMapping}

	// Inject OPENAI_API_KEY if set on the host.
	if a.apiKeyEnv != "" {
		args = append(args, "-e", "OPENAI_API_KEY="+a.apiKeyEnv)
	}
	// Inject ANTHROPIC_API_KEY if set on the host.
	if a.anthropicKeyEnv != "" {
		args = append(args, "-e", "ANTHROPIC_API_KEY="+a.anthropicKeyEnv)
	}

	// Bind-mount stored credential files when they exist (read-only).
	if _, err := os.Stat(a.storedKeyPath); err == nil {
		args = append(args, "-v", a.storedKeyPath+":/root/.config/codex-dock/apikey:ro")
	}
	if _, err := os.Stat(a.anthropicKeyPath); err == nil {
		args = append(args, "-v", a.anthropicKeyPath+":/root/.config/codex-dock/anthropic-apikey:ro")
	}
	if _, err := os.Stat(a.oauthJSONPath); err == nil {
		args = append(args, "-v", a.oauthJSONPath+":/root/.codex/auth.json:ro")
	}
	if _, err := os.Stat(a.claudeCredsPath); err == nil {
		args = append(args, "-v", a.claudeCredsPath+":/root/.claude/.credentials.json:ro")
	}

	// Image and command — CMD overrides the Dockerfile default listen address.
	args = append(args, a.image, "proxy", "serve", "--listen", a.listenAddr)
	if a.adminSecret != "" {
		args = append(args, "--admin-secret", a.adminSecret)
	}

	return args
}

func ensureBridgeNetwork(ctx context.Context, networkName, bridgeName string) error {
	inspect := exec.CommandContext(ctx, "docker", "network", "inspect", networkName)
	if err := inspect.Run(); err == nil {
		return nil
	}

	args := []string{"network", "create", "--driver", "bridge"}
	if bridgeName != "" {
		args = append(args, "--opt", "com.docker.network.bridge.name="+bridgeName)
	}
	args = append(args, networkName)

	create := exec.CommandContext(ctx, "docker", args...)
	create.Stdout = os.Stdout
	create.Stderr = os.Stderr
	if err := create.Run(); err != nil {
		return fmt.Errorf("ensuring docker network %q: %w", networkName, err)
	}
	return nil
}

// resolveProxyDockerfile returns the auth-proxy Dockerfile path and build
// context to use.
// Priority: explicit -f flag > auth-proxy.Dockerfile /
// docker/auth-proxy.Dockerfile in CWD > config-dir default.
// The build context is always "." (CWD) because the proxy image compiles Go
// source from the repo root.
func resolveProxyDockerfile(flagValue string) (string, string, error) {
	if flagValue != "" {
		return flagValue, ".", nil
	}

	// Check well-known locations relative to the current directory.
	// docker/proxy/Dockerfile is the current layout; the older flat names are
	// kept for backward compatibility with existing checkouts.
	for _, p := range []string{"docker/proxy/Dockerfile", "auth-proxy.Dockerfile", "docker/auth-proxy.Dockerfile"} {
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

// ensureProxyDockerfile writes the embedded auth-proxy.Dockerfile into dir
// if not already present.
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
func executeProxyBuild(ctx context.Context, tag, dockerfile, buildCtx string) error {
	fmt.Printf("Building proxy image %s from %s...\n", tag, dockerfile)
	c := exec.CommandContext(ctx, "docker", "build", "-t", tag, "-f", dockerfile, buildCtx)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("docker build failed: %w", err)
	}
	fmt.Printf("Proxy image %s built successfully.\n", tag)
	return nil
}
