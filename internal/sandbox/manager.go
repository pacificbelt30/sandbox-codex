package sandbox

import (
	"context"
	crand "crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/pacificbelt30/codex-dock/internal/authproxy"
	"github.com/pacificbelt30/codex-dock/internal/network"
	"golang.org/x/term"
)

const (
	labelPrefix  = "codex-dock."
	labelManaged = labelPrefix + "managed"
	labelBranch  = labelPrefix + "branch"
	labelTask    = labelPrefix + "task"
)

// ManagerConfig holds configuration for the sandbox manager.
type ManagerConfig struct {
	Proxy   authproxy.Service
	Network *network.Manager
	Verbose bool
	Debug   bool
}

// Manager handles container lifecycle for codex-dock workers.
type Manager struct {
	cli     *client.Client
	proxy   authproxy.Service
	network *network.Manager
	verbose bool
	debug   bool
}

// NewManager creates a new Manager connected to the local Docker daemon.
func NewManager(cfg ManagerConfig) (*Manager, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("connecting to Docker: %w", err)
	}
	return &Manager{
		cli:     cli,
		proxy:   cfg.Proxy,
		network: cfg.Network,
		verbose: cfg.Verbose,
		debug:   cfg.Debug,
	}, nil
}

// Run creates and starts a sandboxed Codex container.
func (m *Manager) Run(opts RunOptions) (string, error) {
	ctx := context.Background()

	name := opts.Name
	if name == "" {
		// Pick a name whose container and per-worker network are both free, so two
		// workers never end up sharing one Internal network (which would break
		// isolation) when generated names collide.
		name = pickUniqueName(generateName, m.nameTaken, 12)
	}

	// Determine workspace path
	workspace := opts.ProjectDir
	if opts.WorktreePath != "" {
		workspace = opts.WorktreePath
	}

	absWorkspace, err := absolutePath(workspace)
	if err != nil {
		return "", fmt.Errorf("resolving workspace path: %w", err)
	}

	// Build package install script
	var allPkgs []Package
	for _, spec := range opts.Packages {
		allPkgs = append(allPkgs, ParsePackage(spec))
	}
	if opts.PkgFile != "" {
		filePkgs, err := LoadPackageFile(opts.PkgFile)
		if err != nil {
			return "", fmt.Errorf("loading package file: %w", err)
		}
		allPkgs = append(allPkgs, filePkgs...)
	}
	installScript := BuildInstallScript(allPkgs)

	// Build environment variables (issues the auth token as a side effect).
	env, err := m.buildEnv(name, opts, installScript)
	if err != nil {
		return "", err
	}

	mounts := []mount.Mount{
		{
			Type:   mount.TypeBind,
			Source: absWorkspace,
			Target: "/workspace",
			BindOptions: &mount.BindOptions{
				Propagation: mount.PropagationRPrivate,
			},
		},
	}
	// Note: auth.json is NOT bind-mounted even in OAuth mode.
	// The Auth Proxy provides only the access_token to the container via
	// the /token endpoint, keeping the refresh_token on the host (F-AUTH-01).

	// Container labels for identification
	labels := map[string]string{
		labelManaged: "true",
		labelBranch:  opts.Branch,
		labelTask:    opts.Task,
	}

	if opts.ReadOnly {
		mounts[0].ReadOnly = true
	}

	// Per-worker Internal network: create it and attach the proxy so the worker
	// can reach the proxy/router but nothing else (no other worker, host, or
	// direct internet). Enforced entirely by Docker — no iptables/sudo.
	netName := network.WorkerNetworkName(name)
	if m.network != nil {
		if err := m.network.EnsureWorkerNetwork(name); err != nil {
			return "", fmt.Errorf("creating worker network: %w", err)
		}
		if proxyName := m.proxyContainerName(); proxyName != "" {
			if err := m.network.ConnectProxy(name, proxyName); err != nil {
				return "", fmt.Errorf("connecting proxy to worker network: %w", err)
			}
		}
	}

	// Security: drop all capabilities, non-root user, isolated Internal network.
	hostConfig := buildHostConfig(mounts, netName)

	// Build codex command
	codexArgs := buildCodexArgs(opts)

	containerConfig := &container.Config{
		Image:        opts.Image,
		Env:          env,
		WorkingDir:   "/workspace",
		Labels:       labels,
		Tty:          !opts.Detach,
		OpenStdin:    !opts.Detach,
		AttachStdin:  !opts.Detach,
		AttachStdout: !opts.Detach,
		AttachStderr: !opts.Detach,
		User:         opts.ContainerUser,
	}
	if opts.ShellMode {
		// Override the Dockerfile ENTRYPOINT so Docker runs bash directly
		// instead of "/entrypoint.sh /bin/bash".
		containerConfig.Entrypoint = []string{"/bin/bash"}
	} else {
		containerConfig.Cmd = []string{"/entrypoint.sh"}
	}
	_ = codexArgs // passed via env

	resp, err := m.cli.ContainerCreate(ctx, containerConfig, hostConfig, nil, nil, name)
	if err != nil {
		return "", fmt.Errorf("creating container %s: %w", name, err)
	}

	if m.verbose {
		fmt.Printf("Container created: %s (%s)\n", name, resp.ID[:12])
	}

	if err := m.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return "", fmt.Errorf("starting container %s: %w", name, err)
	}

	if !opts.Detach {
		// Attach stdio
		go m.attachIO(ctx, resp.ID)
	}

	return resp.ID, nil
}

// Start (re)starts a stopped container by ID. Used by the TUI to restart exited workers (F-UI-02).
func (m *Manager) Start(containerID string) error {
	return m.cli.ContainerStart(context.Background(), containerID, container.StartOptions{})
}

// Wait returns a channel that receives nil when the container exits cleanly, or an error.
func (m *Manager) Wait(containerID string) <-chan error {
	ch := make(chan error, 1)
	go func() {
		statusCh, errCh := m.cli.ContainerWait(context.Background(), containerID, container.WaitConditionNotRunning)
		select {
		case err := <-errCh:
			ch <- err
		case status := <-statusCh:
			if status.StatusCode != 0 {
				ch <- fmt.Errorf("container exited with code %d", status.StatusCode)
			} else {
				ch <- nil
			}
		}
	}()
	return ch
}

// Stop gracefully stops a container by ID and revokes its auth token (F-AUTH-04).
func (m *Manager) Stop(containerID string, timeoutSec int) error {
	ctx := context.Background()
	// Revoke token before stopping so it cannot be used after this point.
	if m.proxy != nil {
		if name, err := m.resolveNameByID(ctx, containerID); err == nil {
			m.proxy.RevokeToken(name)
		}
	}
	return m.cli.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeoutSec})
}

// StopByName stops a container by name and revokes its auth token (F-AUTH-04).
func (m *Manager) StopByName(name string, timeoutSec int) error {
	// Revoke token immediately; Stop() will also attempt but name resolution is cheaper here.
	if m.proxy != nil {
		m.proxy.RevokeToken(name)
	}
	id, err := m.resolveID(name)
	if err != nil {
		return err
	}
	ctx := context.Background()
	return m.cli.ContainerStop(ctx, id, container.StopOptions{Timeout: &timeoutSec})
}

// RevokeToken revokes the auth token for a container identified by ID or name.
// This should be called when a container exits naturally (without Stop being called).
func (m *Manager) RevokeToken(containerID string) {
	if m.proxy == nil {
		return
	}
	ctx := context.Background()
	if name, err := m.resolveNameByID(ctx, containerID); err == nil {
		m.proxy.RevokeToken(name)
	}
}

// ImageExists reports whether the named image tag is present in the local Docker daemon.
func (m *Manager) ImageExists(tag string) (bool, error) {
	_, err := m.cli.ImageInspect(context.Background(), tag)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Remove removes a container by ID and tears down its per-worker network.
func (m *Manager) Remove(containerID string, force bool) error {
	ctx := context.Background()
	name, _ := m.resolveNameByID(ctx, containerID)
	if err := m.cli.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: force}); err != nil {
		return err
	}
	m.cleanupWorkerNetwork(name)
	return nil
}

// RemoveByName removes a container by name and tears down its per-worker network.
func (m *Manager) RemoveByName(name string, force bool) error {
	id, err := m.resolveID(name)
	if err != nil {
		return err
	}
	if err := m.cli.ContainerRemove(context.Background(), id, container.RemoveOptions{Force: force}); err != nil {
		return err
	}
	m.cleanupWorkerNetwork(name)
	return nil
}

// List returns all codex-dock managed workers.
func (m *Manager) List(all bool) ([]Worker, error) {
	ctx := context.Background()
	f := filters.NewArgs()
	f.Add("label", labelManaged+"=true")

	containers, err := m.cli.ContainerList(ctx, container.ListOptions{
		All:     all,
		Filters: f,
	})
	if err != nil {
		return nil, fmt.Errorf("listing containers: %w", err)
	}

	workers := make([]Worker, 0, len(containers))
	for _, c := range containers {
		w := Worker{
			ID:     c.ID,
			Name:   strings.TrimPrefix(c.Names[0], "/"),
			Status: c.State,
			Image:  c.Image,
			Branch: c.Labels[labelBranch],
			Task:   c.Labels[labelTask],
		}
		if c.Created > 0 {
			t := time.Unix(c.Created, 0)
			w.StartedAt = &t
		}
		workers = append(workers, w)
	}
	return workers, nil
}

// Logs streams container logs to opts.Output (defaults to os.Stdout).
func (m *Manager) Logs(opts LogOptions) error {
	ctx := context.Background()
	id, err := m.resolveID(opts.Name)
	if err != nil {
		return err
	}

	tail := strconv.Itoa(opts.Tail)
	rc, err := m.cli.ContainerLogs(ctx, id, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       tail,
		Follow:     opts.Follow,
		Timestamps: false,
	})
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()

	out := opts.Output
	if out == nil {
		out = os.Stdout
	}
	_, err = io.Copy(out, rc)
	return err
}

// resolveNameByID returns the container name (without leading '/') for a given ID.
func (m *Manager) resolveNameByID(ctx context.Context, containerID string) (string, error) {
	info, err := m.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return "", err
	}
	return strings.TrimPrefix(info.Name, "/"), nil
}

func (m *Manager) resolveID(nameOrID string) (string, error) {
	ctx := context.Background()
	containers, err := m.cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("label", labelManaged+"=true")),
	})
	if err != nil {
		return "", err
	}
	for _, c := range containers {
		if strings.HasPrefix(c.ID, nameOrID) {
			return c.ID, nil
		}
		for _, n := range c.Names {
			if strings.TrimPrefix(n, "/") == nameOrID {
				return c.ID, nil
			}
		}
	}
	return "", fmt.Errorf("container not found: %s", nameOrID)
}

func (m *Manager) attachIO(ctx context.Context, containerID string) {
	resp, err := m.cli.ContainerAttach(ctx, containerID, container.AttachOptions{
		Stdin:  true,
		Stdout: true,
		Stderr: true,
		Stream: true,
	})
	if err != nil {
		return
	}
	defer resp.Close()

	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		oldState, err := term.MakeRaw(fd)
		if err == nil {
			defer term.Restore(fd, oldState) //nolint:errcheck
		}

		// Set initial PTY size
		if w, h, err := term.GetSize(fd); err == nil {
			_ = m.cli.ContainerResize(ctx, containerID, container.ResizeOptions{
				Width:  uint(w),
				Height: uint(h),
			})
		}

		// Forward terminal resize events (SIGWINCH) to the container PTY
		resizeCh := make(chan os.Signal, 1)
		signal.Notify(resizeCh, syscall.SIGWINCH)
		defer signal.Stop(resizeCh)
		go func() {
			for range resizeCh {
				w, h, err := term.GetSize(fd)
				if err == nil {
					_ = m.cli.ContainerResize(ctx, containerID, container.ResizeOptions{
						Width:  uint(w),
						Height: uint(h),
					})
				}
			}
		}()
	}

	// Pass stdin bytes to the container as-is.  Ctrl+Z (0x1a) is forwarded
	// to the container's PTY where the in-container bash handles job control:
	// codex is suspended and the bash prompt appears inside the container.
	go func() { _, _ = io.Copy(resp.Conn, os.Stdin) }()
	_, _ = io.Copy(os.Stdout, resp.Reader)
}

// buildEnv assembles the container environment, issuing a short-lived auth token
// from the proxy when one is configured. The agent (codex/claude/shell) controls
// which provider's proxy variables are injected:
//   - codex / shell : OpenAI/Codex variables (CODEX_*, OPENAI_BASE_URL)
//   - claude / shell: Anthropic variables (ANTHROPIC_*) when the proxy can serve them
//
// Containers always receive only a placeholder token; the proxy injects the real
// credential on every outbound request.
func (m *Manager) buildEnv(name string, opts RunOptions, installScript string) ([]string, error) {
	env := []string{
		"CODEX_SANDBOX=1",
		"DOCK_AGENT=" + string(opts.Agent),
	}

	if m.proxy != nil {
		token, err := m.proxy.IssueToken(name, opts.TokenTTL)
		if err != nil {
			return nil, fmt.Errorf("issuing auth token: %w", err)
		}
		// ContainerEndpoint() returns the auth proxy URL reachable from workers
		// over the per-worker Internal network via Docker DNS (codex-auth-proxy).
		containerProxyURL := m.proxy.ContainerEndpoint()

		wantCodex := opts.Agent == AgentCodex || opts.Agent == AgentNone
		wantClaude := opts.Agent == AgentClaude || opts.Agent == AgentNone

		if wantCodex {
			env = append(env,
				"CODEX_AUTH_PROXY_URL="+containerProxyURL,
				"CODEX_TOKEN="+token,
				// Route all Responses API traffic through the proxy so the proxy can
				// substitute credentials. OPENAI_BASE_URL overrides the Codex CLI default
				// (https://chatgpt.com/backend-api/codex) in all auth modes.
				"OPENAI_BASE_URL="+containerProxyURL+"/v1",
			)
			// In OAuth mode, redirect Codex CLI's token refresh calls to the proxy.
			// The proxy substitutes the host's real refresh_token so it never reaches
			// the container. The short-lived token is embedded as ?cdx= for authentication
			// because Codex CLI does not add custom headers to refresh requests.
			if m.proxy.IsOAuthMode() {
				env = append(env,
					"CODEX_REFRESH_TOKEN_URL_OVERRIDE="+containerProxyURL+"/oauth/token?cdx="+token,
				)
			}
		}

		// Claude Code is fully env-driven: ANTHROPIC_BASE_URL points at the proxy's
		// /anthropic route and ANTHROPIC_API_KEY carries the placeholder token. The
		// proxy injects the host's real API key or OAuth bearer on outbound requests.
		if wantClaude && m.proxy.IsAnthropicMode() {
			env = append(env,
				"ANTHROPIC_BASE_URL="+containerProxyURL+"/anthropic",
				"ANTHROPIC_API_KEY="+token,
			)
		}

		// Route general egress (git/npm/pip/curl) through the proxy/router via the
		// standard HTTP(S)_PROXY vars. NO_PROXY excludes the proxy host itself so
		// token fetches and the API base URLs reach it directly instead of looping
		// through the CONNECT forward proxy. Skipped when --no-internet is set, which
		// leaves only the proxy's credential-injecting API routes reachable.
		if !opts.NoInternet {
			proxyHost := proxyEndpointHost(containerProxyURL)
			env = append(env,
				"HTTP_PROXY="+containerProxyURL,
				"HTTPS_PROXY="+containerProxyURL,
				"http_proxy="+containerProxyURL,
				"https_proxy="+containerProxyURL,
				"NO_PROXY="+proxyHost+",localhost,127.0.0.1",
				"no_proxy="+proxyHost+",localhost,127.0.0.1",
			)
		}

		if m.debug {
			fmt.Fprintf(os.Stderr, "debug: issued token for %s (TTL=%ds, agent=%q)\n", name, opts.TokenTTL, opts.Agent)
		}
	}

	if opts.Task != "" {
		env = append(env, "CODEX_TASK="+opts.Task)
	}
	if opts.Model != "" {
		env = append(env, "CODEX_MODEL="+opts.Model)
	}
	if opts.ApprovalMode != "" && opts.ApprovalMode != ApprovalModeSuggest {
		env = append(env, "CODEX_APPROVAL_MODE="+string(opts.ApprovalMode))
	}
	if opts.AgentsMD != "" {
		env = append(env, "CODEX_AGENTS_MD="+opts.AgentsMD)
	}
	if installScript != "" {
		env = append(env, "CODEX_INSTALL_SCRIPT="+installScript)
	}

	// When a custom container user is specified, the uid may not exist in the
	// image's /etc/passwd, causing Docker to set HOME="/".
	// Use a writable non-workspace home to avoid permission errors and avoid creating extra directories under /workspace.
	if opts.ContainerUser != "" {
		env = append(env, "HOME=/var/tmp/codex-home")
	}

	return env, nil
}

// proxyContainerName returns the name of the auth proxy container, derived from
// the proxy's container endpoint (e.g. http://codex-auth-proxy:18080 →
// "codex-auth-proxy"). This is the container the per-worker Internal networks are
// attached to. Returns "" when no proxy or endpoint is configured.
func (m *Manager) proxyContainerName() string {
	if m.proxy == nil {
		return ""
	}
	return proxyEndpointHost(m.proxy.ContainerEndpoint())
}

// proxyEndpointHost extracts the hostname from a proxy endpoint URL.
func proxyEndpointHost(endpoint string) string {
	u, err := url.Parse(endpoint)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// nameTaken reports whether a worker name is already in use by an existing
// container or by an existing per-worker network. Errors are treated as
// "available" so transient lookup failures don't block name selection.
func (m *Manager) nameTaken(name string) bool {
	if _, err := m.resolveID(name); err == nil {
		return true
	}
	if m.network != nil {
		if exists, err := m.network.WorkerNetworkExists(name); err == nil && exists {
			return true
		}
	}
	return false
}

// pickUniqueName returns the first name produced by gen that is not reported
// taken. After maxAttempts collisions it appends a short random suffix, which is
// itself re-checked so the fallback name is verified free too.
func pickUniqueName(gen func() string, taken func(string) bool, maxAttempts int) string {
	for i := 0; i < maxAttempts; i++ {
		n := gen()
		if !taken(n) {
			return n
		}
	}
	base := gen()
	for i := 0; i < maxAttempts; i++ {
		n := base + "-" + randomSuffix()
		if !taken(n) {
			return n
		}
	}
	// Extremely unlikely; return a suffixed name regardless.
	return base + "-" + randomSuffix()
}

// randomSuffix returns a short random hex string for disambiguating names.
func randomSuffix() string {
	b := make([]byte, 4)
	if _, err := crand.Read(b); err != nil {
		// crypto/rand should not fail; fall back to a timestamp-based value.
		return fmt.Sprintf("%08x", time.Now().UnixNano()&0xffffffff)
	}
	return hex.EncodeToString(b)
}

// cleanupWorkerNetwork removes a worker's Internal network after its container is
// gone. RemoveWorkerNetwork force-disconnects any remaining endpoints (notably the
// multi-homed proxy), so this works even when the Manager has no proxy reference
// (e.g. the `rm` command / TUI). Failures are reported but not fatal.
func (m *Manager) cleanupWorkerNetwork(name string) {
	if m.network == nil || name == "" {
		return
	}
	if err := m.network.RemoveWorkerNetwork(name); err != nil {
		fmt.Fprintf(os.Stderr, "warning: removing worker network for %s: %v\n", name, err)
	}
}

func buildCodexArgs(opts RunOptions) []string {
	args := []string{"codex"}
	switch opts.ApprovalMode {
	case ApprovalModeAutoEdit:
		args = append(args, "--ask-for-approval", "unless-allow-listed")
	case ApprovalModeFullAuto:
		args = append(args, "--ask-for-approval", "never")
	case ApprovalModeDanger:
		args = append(args, "--dangerously-bypass-approvals-and-sandbox")
	}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.Task != "" {
		args = append(args, opts.Task)
	}
	return args
}

func absolutePath(path string) (string, error) {
	if len(path) > 0 && path[0] == '/' {
		return path, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return wd + "/" + path, nil
}

// buildHostConfig constructs the container HostConfig with security defaults.
// netName is the per-worker Internal network the container is attached to; the
// worker reaches the proxy over this network via Docker embedded DNS
// (codex-auth-proxy), so no host.docker.internal/host-gateway alias is needed.
func buildHostConfig(mounts []mount.Mount, netName string) *container.HostConfig {
	return &container.HostConfig{
		NetworkMode: container.NetworkMode(netName),
		Mounts:      mounts,
		CapDrop:     []string{"ALL"},
		SecurityOpt: []string{"no-new-privileges:true"},
		Resources: container.Resources{
			PidsLimit: int64ptr(512),
		},
	}
}

func int64ptr(i int64) *int64 { return &i }
