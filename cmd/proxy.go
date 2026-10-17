package cmd

import (
	"context"
	"fmt"
	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/pacificbelt30/codex-dock/internal/engine"
	"os"
	"path/filepath"
	"strconv"

	dockerdefaults "github.com/pacificbelt30/codex-dock/docker"
	"github.com/pacificbelt30/codex-dock/internal/authproxy"
	"github.com/pacificbelt30/codex-dock/internal/network"
	"github.com/pacificbelt30/codex-dock/internal/networkpolicy"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	defaultProxyContainerName     = "codex-auth-proxy"
	defaultHTTPProxyContainerName = "codex-http-proxy"
	defaultAuthDataPort           = 18080
	defaultAuthAdminPort          = 18081
	defaultHTTPProxyPort          = 18082
	proxyLayoutLabel              = "io.codex-dock.proxy-layout"
	proxyLayoutVersion            = "split-v1"
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
		applyForwardPolicyDefaults(cmd)
		p, err := authproxy.NewProxy(authproxy.Config{
			TokenTTL:        3600,
			Verbose:         verbose,
			Role:            proxyRole,
			ListenAddr:      proxyListenAddr,
			AdminListenAddr: proxyAdminListenAddr,
			AdminSecret:     proxyAdminSecret,
			ForwardPolicy:   forwardPolicyRules(),
			BlockPrivate:    proxyBlockPrivate,
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
	Short: "Build the auth proxy OCI image with BuildKit",
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
	Short: "Run the auth proxy on containerd",
	Long: `Run the auth proxy as a detached containerd task.

Credentials are automatically bound from the host into the container:
  - OPENAI_API_KEY env var  → injected as -e OPENAI_API_KEY=<value>
  - ~/.config/codex-dock/apikey → bind-mounted read-only (API key file)
  - ~/.codex/auth.json           → bind-mounted read-only (OAuth/ChatGPT)

At least one credential source must be configured before running.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		applyForwardPolicyDefaults(cmd)
		return startProxyContainer(cmd.Context())
	},
}

// proxyContainerInfo returns the containerd task status and the codex-dock proxy layout
// version of the named container. Empty values mean the container does not
// exist. Containers created before layout versioning have an empty version and
// must be recreated: in particular, the old single-proxy layout publishes
// port 18080 while current clients connect to the split admin port at 18081.
func proxyContainerInfo(ctx context.Context, name string) (state, layout string) {
	c, err := engine.NewClient()
	if err != nil {
		return "", ""
	}
	defer func() { _ = c.Close() }()
	v, err := c.ContainerInspect(ctx, name)
	if err != nil || v.State == nil || v.Config == nil {
		return "", ""
	}
	return v.State.Status, v.Config.Labels[proxyLayoutLabel]
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
	rules := forwardPolicyRules()
	rules.BlockPrivate = true
	if _, err := networkpolicy.Compile(rules); err != nil {
		return fmt.Errorf("invalid egress policy: %w", err)
	}
	image := viper.GetString("proxy_image")
	if err := ensureBridgeNetwork(ctx, proxyRunNetwork, network.ProxyBridgeName); err != nil {
		return err
	}
	if err := ensureProxyImage(ctx, image); err != nil {
		return err
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
		name:          proxyRunHTTPName,
		role:          authproxy.RoleEgress,
		networkName:   proxyRunNetwork,
		image:         image,
		listenAddr:    fmt.Sprintf("0.0.0.0:%d", proxyRunHTTPPort),
		blockPrivate:  true,
		forwardAllow:  proxyForwardAllow,
		forwardPolicy: proxyForwardRules,
	}
	return ensureProxyContainer(ctx, egress)
}

// ensureProxyImage builds the proxy image when it is not present locally.
func ensureProxyImage(ctx context.Context, image string) error {
	c, err := engine.NewClient()
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	if _, err := c.ImageInspect(ctx, image); err == nil {
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
	c, err := engine.NewClient()
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	v, err := c.ContainerInspect(ctx, spec.name)
	if err != nil && !errdefs.IsNotFound(err) {
		return err
	}
	if err == nil {
		if v.Config == nil || v.State == nil {
			return fmt.Errorf("incomplete proxy record")
		}
		if v.Config.Labels[proxyLayoutLabel] != proxyLayoutVersion {
			if err = c.ContainerRemove(ctx, spec.name, container.RemoveOptions{Force: true}); err != nil {
				return err
			}
		} else {
			if spec.role == authproxy.RoleEgress {
				if err = checkProxyPolicy(v.Config.Labels[proxyPolicyLabel], spec); err != nil {
					return err
				}
			}
			if v.State.Running {
				return nil
			}
			return c.ContainerStart(ctx, spec.name, container.StartOptions{})
		}
	}
	cfg, host, err := proxyConfiguration(spec)
	if err != nil {
		return err
	}
	created, err := c.ContainerCreate(ctx, cfg, host, nil, nil, spec.name)
	if err != nil {
		return err
	}
	return c.ContainerStart(ctx, created.ID, container.StartOptions{})
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
			if err := stopProxy(cmd.Context(), name); err != nil {
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
			fmt.Printf("Removing proxy container %q...\n", name)

			if err := removeProxy(cmd.Context(), name, proxyRmForce); err != nil {
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
	bindForwardPolicyFlags(proxyServeCmd)
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
	proxyRunCmd.Flags().StringVar(&proxyRunNetwork, "network", network.ProxyNetworkName, "Node egress network to attach the proxy containers to")
	proxyRunCmd.Flags().StringVar(&proxyAdminSecret, "admin-secret", "", "admin secret for /admin/* endpoints")
	bindForwardPolicyFlags(proxyRunCmd)

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
	forwardPolicy   networkpolicy.Rules
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
	args := []string{
		"run", "-d", "--name", a.name,
		"--label", proxyLayoutLabel + "=" + proxyLayoutVersion,
		"--network", a.networkName,
	}
	if a.role == authproxy.RoleEgress {
		args = append(args, "--label", proxyPolicyLabel+"="+forwardPolicyID(a))
	}
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
	rules := effectiveForwardRules(a)
	for _, d := range rules.AllowDomains {
		args = append(args, "--forward-allow-domain", d)
	}
	for _, setting := range []struct {
		flag   string
		values []string
	}{
		{"--forward-deny-domain", rules.DenyDomains},
		{"--forward-allow-cidr", rules.AllowCIDRs},
		{"--forward-deny-cidr", rules.DenyCIDRs},
	} {
		for _, value := range setting.values {
			args = append(args, setting.flag, value)
		}
	}
	for _, setting := range []struct {
		flag   string
		values []int
	}{
		{"--forward-allow-port", rules.AllowPorts},
		{"--forward-deny-port", rules.DenyPorts},
	} {
		for _, value := range setting.values {
			args = append(args, setting.flag, strconv.Itoa(value))
		}
	}

	return args
}

func ensureBridgeNetwork(ctx context.Context, networkName, bridgeName string) error {
	return ensureNodeBridge(ctx, networkName)
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
// --progress=plain forces BuildKit to stream every step's full output line-by-line
// instead of collapsing it into a live-updating TTY view, so an auto-build
// triggered by `run` never looks stalled while it's actually still working.
func executeProxyBuild(ctx context.Context, tag, dockerfile, buildCtx string) error {
	return executeBuild(ctx, tag, dockerfile, buildCtx)
}
