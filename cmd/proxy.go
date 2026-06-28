package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	dockerdefaults "github.com/pacificbelt30/codex-dock/docker"
	"github.com/pacificbelt30/codex-dock/internal/authproxy"
	"github.com/pacificbelt30/codex-dock/internal/network"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	defaultProxyContainerName     = "codex-auth-proxy"
	defaultHTTPProxyContainerName = "codex-http-proxy"
	defaultAuthDataPort           = 18080
	defaultAuthAdminPort          = 18081
	defaultHTTPProxyPort          = 18082
)

var (
	proxyListenAddr      string
	proxyAdminListenAddr string
	proxyAdminSecret     string
	proxyForwardAllow    []string
	proxyRole            string
	proxyBlockPrivate    bool

	proxyBuildTag        string
	proxyBuildDockerfile string

	proxyRunName      string
	proxyRunPort      int
	proxyRunAdminPort int
	proxyRunHTTPName  string
	proxyRunHTTPPort  int
	proxyRunNetwork   string

	proxyStopName string

	proxyRmName  string
	proxyRmForce bool
)

// managedProxyNames returns the default proxy container names codex-dock manages.
func managedProxyNames() []string {
	return []string{defaultProxyContainerName, defaultHTTPProxyContainerName}
}

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
			TokenTTL:            3600,
			Verbose:             verbose,
			Role:                proxyRole,
			ListenAddr:          proxyListenAddr,
			AdminListenAddr:     proxyAdminListenAddr,
			AdminSecret:         proxyAdminSecret,
			ForwardAllowDomains: proxyForwardAllow,
			BlockPrivate:        proxyBlockPrivate,
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
		if !cmd.Flags().Changed("forward-allow-domain") && viper.IsSet("proxy.forward_allow_domains") {
			proxyForwardAllow = viper.GetStringSlice("proxy.forward_allow_domains")
		}
		return startProxyContainer(cmd.Context())
	},
}

// proxyContainerState returns the Docker status string of the named container
// ("running", "exited", "created", ...), or "" when it does not exist.
func proxyContainerState(ctx context.Context, name string) string {
	out, err := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{.State.Status}}", name).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// startProxyContainer ensures BOTH proxy containers are running:
//   - codex-auth-proxy (RoleAuth): credential-injecting reverse routes + token +
//     admin. Data-plane internal; admin published to host loopback (egress IP).
//   - codex-http-proxy (RoleEgress): general-egress forward proxy, no credentials,
//     private/LAN blocked.
//
// It builds the proxy image / egress network when missing and restarts existing
// (stopped) containers. Shared by `proxy run` and `codex-dock run`'s auto-start.
func startProxyContainer(ctx context.Context) error {
	image := viper.GetString("proxy_image")
	if err := ensureBridgeNetwork(ctx, proxyRunNetwork, network.ProxyBridgeName); err != nil {
		return err
	}
	if err := ensureProxyImage(ctx, image); err != nil {
		return err
	}
	if len(proxyForwardAllow) == 0 && viper.IsSet("proxy.forward_allow_domains") {
		proxyForwardAllow = viper.GetStringSlice("proxy.forward_allow_domains")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}

	// 1) Auth proxy: holds credentials, admin published, no forwarding.
	auth := proxyRunArgs{
		name:             proxyRunName,
		role:             authproxy.RoleAuth,
		networkName:      proxyRunNetwork,
		image:            image,
		listenAddr:       fmt.Sprintf("0.0.0.0:%d", proxyRunPort),
		adminPort:        proxyRunAdminPort,
		adminListenAddr:  fmt.Sprintf("%s:%d", authproxy.AdminBindEgress, proxyRunAdminPort),
		adminSecret:      proxyAdminSecret,
		blockPrivate:     true, // defense-in-depth on its fixed upstreams
		mountCreds:       true,
		apiKeyEnv:        os.Getenv("OPENAI_API_KEY"),
		storedKeyPath:    filepath.Join(home, ".config", "codex-dock", "apikey"),
		oauthJSONPath:    filepath.Join(home, ".codex", "auth.json"),
		anthropicKeyEnv:  os.Getenv("ANTHROPIC_API_KEY"),
		anthropicKeyPath: filepath.Join(home, ".config", "codex-dock", "anthropic-apikey"),
		claudeCredsPath:  filepath.Join(home, ".claude", ".credentials.json"),
	}
	if err := ensureProxyContainer(ctx, auth); err != nil {
		return err
	}

	// 2) Egress proxy: forward-only, no credentials, LAN blocked.
	egress := proxyRunArgs{
		name:         proxyRunHTTPName,
		role:         authproxy.RoleEgress,
		networkName:  proxyRunNetwork,
		image:        image,
		listenAddr:   fmt.Sprintf("0.0.0.0:%d", proxyRunHTTPPort),
		blockPrivate: true,
		forwardAllow: proxyForwardAllow,
	}
	return ensureProxyContainer(ctx, egress)
}

// ensureProxyImage builds the proxy image when it is not present locally.
func ensureProxyImage(ctx context.Context, image string) error {
	if exec.CommandContext(ctx, "docker", "image", "inspect", image).Run() == nil {
		return nil
	}
	fmt.Printf("Proxy image %s not found locally, building...\n", image)
	dockerfile, buildCtx, err := resolveProxyDockerfile("")
	if err != nil {
		return err
	}
	return executeProxyBuild(ctx, image, dockerfile, buildCtx)
}

// ensureProxyContainer (re)starts one proxy container from its spec: a running
// container is left alone, a stopped one is started, and a missing one is created.
func ensureProxyContainer(ctx context.Context, spec proxyRunArgs) error {
	switch proxyContainerState(ctx, spec.name) {
	case "running":
		fmt.Printf("Proxy container %q is already running.\n", spec.name)
		return nil
	case "":
		// Not found — create below.
	default:
		fmt.Printf("Starting existing proxy container %q...\n", spec.name)
		c := exec.CommandContext(ctx, "docker", "start", spec.name)
		c.Stdout, c.Stderr = os.Stdout, os.Stderr
		return c.Run()
	}

	fmt.Printf("Starting proxy container %q (role: %s, image: %s, network: %s)...\n",
		spec.name, spec.role, spec.image, spec.networkName)
	c := exec.CommandContext(ctx, "docker", buildProxyRunArgs(spec)...)
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("docker run %q failed: %w", spec.name, err)
	}
	fmt.Printf("Proxy container %q started.\n", spec.name)
	return nil
}

// proxy stop ----------------------------------------------------------------

var proxyStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the proxy containers (auth + egress)",
	RunE: func(cmd *cobra.Command, args []string) error {
		names := managedProxyNames()
		if cmd.Flags().Changed("name") {
			names = []string{proxyStopName}
		}
		for _, name := range names {
			fmt.Printf("Stopping proxy container %q...\n", name)
			c := exec.CommandContext(cmd.Context(), "docker", "stop", name)
			c.Stdout, c.Stderr = os.Stdout, os.Stderr
			if err := c.Run(); err != nil {
				fmt.Printf("  error: %v\n", err)
			}
		}
		return nil
	},
}

// proxy rm ------------------------------------------------------------------

var proxyRmCmd = &cobra.Command{
	Use:   "rm",
	Short: "Remove the proxy containers (auth + egress)",
	RunE: func(cmd *cobra.Command, args []string) error {
		names := managedProxyNames()
		if cmd.Flags().Changed("name") {
			names = []string{proxyRmName}
		}
		for _, name := range names {
			dockerArgs := []string{"rm"}
			if proxyRmForce {
				dockerArgs = append(dockerArgs, "--force")
			}
			dockerArgs = append(dockerArgs, name)
			fmt.Printf("Removing proxy container %q...\n", name)
			c := exec.CommandContext(cmd.Context(), "docker", dockerArgs...)
			c.Stdout, c.Stderr = os.Stdout, os.Stderr
			if err := c.Run(); err != nil {
				fmt.Printf("  error: %v\n", err)
			}
		}
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
	proxyServeCmd.Flags().StringVar(&proxyRole, "role", authproxy.RoleAuth, "Proxy role: \"auth\" (reverse routes + token + admin, no forwarding) or \"egress\" (forward proxy only, no credentials)")
	proxyServeCmd.Flags().StringVar(&proxyListenAddr, "listen", "0.0.0.0:18080", "worker-facing listen address")
	proxyServeCmd.Flags().StringVar(&proxyAdminListenAddr, "admin-listen", "", "separate listen address for /admin/* endpoints (auth role). Use host \"egress\" (e.g. egress:18081) to bind the container's egress IP so workers cannot reach it")
	proxyServeCmd.Flags().StringVar(&proxyAdminSecret, "admin-secret", "", "admin secret for /admin/* endpoints")
	proxyServeCmd.Flags().StringArrayVar(&proxyForwardAllow, "forward-allow-domain", nil, "Restrict the forward proxy (egress role) to these domains and subdomains (repeatable; default: allow all)")
	proxyServeCmd.Flags().BoolVar(&proxyBlockPrivate, "block-private", false, "Refuse outbound connections to private/LAN/link-local addresses (RFC1918, 127/8, 169.254/16, ULA, CGNAT)")

	// build flags
	proxyBuildCmd.Flags().StringVarP(&proxyBuildTag, "tag", "t", "", "Image tag (default: proxy_image from config)")
	proxyBuildCmd.Flags().StringVarP(&proxyBuildDockerfile, "dockerfile", "f", "", "Path to auth-proxy.Dockerfile")

	// run flags
	proxyRunCmd.Flags().StringVar(&proxyRunName, "name", defaultProxyContainerName, "Auth proxy container name")
	proxyRunCmd.Flags().IntVarP(&proxyRunPort, "port", "p", defaultAuthDataPort, "Auth proxy internal data-plane port (not host-published)")
	proxyRunCmd.Flags().IntVar(&proxyRunAdminPort, "admin-port", defaultAuthAdminPort, "Host-published admin port (bound to 127.0.0.1) for /admin/* endpoints")
	proxyRunCmd.Flags().StringVar(&proxyRunHTTPName, "http-name", defaultHTTPProxyContainerName, "Egress (http) proxy container name")
	proxyRunCmd.Flags().IntVar(&proxyRunHTTPPort, "http-port", defaultHTTPProxyPort, "Egress (http) proxy internal forward port (not host-published)")
	proxyRunCmd.Flags().StringVar(&proxyRunNetwork, "network", network.ProxyNetworkName, "Docker egress network to attach the proxy containers to")
	proxyRunCmd.Flags().StringVar(&proxyAdminSecret, "admin-secret", "", "admin secret for /admin/* endpoints")
	proxyRunCmd.Flags().StringArrayVar(&proxyForwardAllow, "forward-allow-domain", nil, "Restrict the egress forward proxy to these domains and subdomains (repeatable; default: allow all)")

	// stop flags
	proxyStopCmd.Flags().StringVar(&proxyStopName, "name", defaultProxyContainerName, "Container name")

	// rm flags
	proxyRmCmd.Flags().StringVar(&proxyRmName, "name", defaultProxyContainerName, "Container name")
	proxyRmCmd.Flags().BoolVarP(&proxyRmForce, "force", "f", false, "Force remove a running container")
}

// proxyRunArgs collects the inputs needed to build the "docker run" command for
// one proxy container (auth or egress role).
type proxyRunArgs struct {
	name            string
	role            string // authproxy.RoleAuth / RoleEgress
	networkName     string
	image           string
	listenAddr      string
	adminPort       int    // >0: publish to 127.0.0.1 (auth only)
	adminListenAddr string // "" disables the admin listener (egress)
	adminSecret     string
	forwardAllow    []string
	blockPrivate    bool
	mountCreds      bool // bind host credentials (auth only)

	// OpenAI / Codex credential sources (used when mountCreds).
	apiKeyEnv     string // OPENAI_API_KEY env value
	storedKeyPath string // ~/.config/codex-dock/apikey
	oauthJSONPath string // ~/.codex/auth.json

	// Anthropic / Claude Code credential sources (used when mountCreds).
	anthropicKeyEnv  string // ANTHROPIC_API_KEY env value
	anthropicKeyPath string // ~/.config/codex-dock/anthropic-apikey
	claudeCredsPath  string // ~/.claude/.credentials.json
}

// buildProxyRunArgs constructs the "docker run" argument list for one proxy
// container. The auth role publishes only its admin port (host loopback) and
// binds the host's credentials; the egress role publishes nothing and mounts no
// credentials. The data-plane / forward port is never host-published — workers
// reach it over their per-worker Docker network.
func buildProxyRunArgs(a proxyRunArgs) []string {
	args := []string{"run", "-d", "--name", a.name, "--network", a.networkName}
	if a.adminPort > 0 {
		args = append(args, "-p", fmt.Sprintf("127.0.0.1:%d:%d", a.adminPort, a.adminPort))
	}

	if a.mountCreds {
		if a.apiKeyEnv != "" {
			args = append(args, "-e", "OPENAI_API_KEY="+a.apiKeyEnv)
		}
		if a.anthropicKeyEnv != "" {
			args = append(args, "-e", "ANTHROPIC_API_KEY="+a.anthropicKeyEnv)
		}
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
	}

	args = append(args, a.image, "proxy", "serve", "--listen", a.listenAddr)
	if a.role != "" {
		args = append(args, "--role", a.role)
	}
	if a.adminListenAddr != "" {
		args = append(args, "--admin-listen", a.adminListenAddr)
	}
	if a.adminSecret != "" {
		args = append(args, "--admin-secret", a.adminSecret)
	}
	if a.blockPrivate {
		args = append(args, "--block-private")
	}
	for _, d := range a.forwardAllow {
		args = append(args, "--forward-allow-domain", d)
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
