package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/pacificbelt30/codex-dock/internal/authproxy"
	"github.com/pacificbelt30/codex-dock/internal/network"
	"github.com/pacificbelt30/codex-dock/internal/sandbox"
	"github.com/pacificbelt30/codex-dock/internal/worktree"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/term"
)

// userMode is the raw value of --user before resolution.
var userMode string

// approvalModeFlag holds the raw value of --approval-mode before validation.
var approvalModeFlag string

// agentFlag holds the raw value of --agent before validation.
var agentFlag string

// fullAutoFlag is a deprecated alias for --approval-mode full-auto.
var fullAutoFlag bool

var runOpts sandbox.RunOptions
var (
	proxyAdminURL       string
	proxyContainerURL   string
	httpProxyURL        string
	runProxyAdminSecret string
	runKeep             bool
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Start a sandboxed Codex worker container",
	Long: `Start a Docker container with Codex CLI isolated from the host environment.
Auth credentials are injected via the Auth Proxy instead of being directly mounted.`,
	RunE: runWorker,
}

func init() {
	rootCmd.AddCommand(runCmd)

	f := runCmd.Flags()
	f.StringVarP(&runOpts.Image, "image", "i", "codex-dock:latest", "Docker image for the sandbox")
	f.StringArrayVarP(&runOpts.Packages, "pkg", "p", nil, "Additional packages to install (apt:<pkg>, pip:<pkg>, npm:<pkg>)")
	f.StringVar(&runOpts.PkgFile, "pkg-file", "", "Path to package definition file (packages.dock)")
	f.StringVarP(&runOpts.ProjectDir, "project", "d", ".", "Project directory to mount as /workspace")
	f.BoolVarP(&runOpts.UseWorktree, "worktree", "w", false, "Use git worktree for isolation")
	f.StringVarP(&runOpts.Branch, "branch", "b", "", "Branch to checkout (requires --worktree)")
	f.BoolVarP(&runOpts.NewBranch, "new-branch", "B", false, "Create new branch (requires --worktree and --branch)")
	f.StringVarP(&runOpts.Name, "name", "n", "", "Container name (auto-generated if omitted)")
	f.StringVar(&agentFlag, "agent", "", `AI agent to launch inside the sandbox.
  ""       interactive shell with auth configured (default; codex and claude both available)
  codex    launch OpenAI Codex CLI
  claude   launch Anthropic Claude Code`)
	f.StringVarP(&runOpts.Task, "task", "t", "", "Initial task prompt for the agent")
	f.StringVar(&approvalModeFlag, "approval-mode", "suggest", `Approval mode for Codex CLI.
  suggest   ask for approval on every action (default, safest)
  auto-edit auto-apply file edits; ask before running shell commands
  full-auto never ask for approval (--ask-for-approval never)
  danger    bypass all approvals and sandbox restrictions
            (--dangerously-bypass-approvals-and-sandbox)
            Docker container isolation provides the safety boundary.`)
	f.BoolVar(&fullAutoFlag, "full-auto", false, "Deprecated: use --approval-mode full-auto")
	if err := f.MarkDeprecated("full-auto", "use --approval-mode full-auto instead"); err != nil {
		panic(err)
	}
	f.StringVarP(&runOpts.Model, "model", "m", "", "Model name to pass to Codex")
	f.BoolVar(&runOpts.ReadOnly, "read-only", false, "Mount project as read-only")
	f.BoolVar(&runOpts.NoInternet, "no-internet", false, "Disable general egress for the worker: do not set HTTP(S)_PROXY, so only the auth proxy's API routes are reachable (no git/npm/pip egress through the router)")
	f.IntVar(&runOpts.TokenTTL, "token-ttl", 3600, "Token TTL in seconds")
	f.StringVar(&runOpts.AgentsMD, "agents-md", "", "Path to additional AGENTS.md")
	f.StringVar(&proxyAdminURL, "proxy-admin-url", "http://127.0.0.1:18081", "External auth proxy admin URL (host-published admin port)")
	f.StringVar(&proxyContainerURL, "proxy-container-url", "http://codex-auth-proxy:18080", "Auth proxy URL reachable from worker containers (API reverse routes)")
	f.StringVar(&httpProxyURL, "http-proxy-url", "http://codex-http-proxy:18082", "General-egress (forward) proxy URL workers use for HTTP(S)_PROXY")
	f.StringVar(&runProxyAdminSecret, "proxy-admin-secret", "", "Admin secret for external auth proxy")
	f.BoolVarP(&runOpts.Detach, "detach", "D", false, "Run container in background")
	f.BoolVar(&runKeep, "keep", false, "Keep the container and its per-worker network after a foreground run exits (default: remove them so networks don't accumulate)")
	f.IntVarP(&runOpts.Parallel, "parallel", "P", 1, "Number of parallel workers")
	f.BoolVarP(&runOpts.ShellMode, "shell", "s", false, "Start a raw bash shell, bypassing entrypoint auth setup (debugging). The default (no --agent) already provides an auth-configured shell.")
	f.StringVar(&userMode, "user", "current", `User to run as inside the container.
	  "current" current command user (uid:gid, default)
	  "codex"   codex user in image (uid:1001)
	  ""        image default user
	  "dir"     project directory owner (uid:gid)
	  "uid"     explicit uid (e.g. "1000" or "1000:1000")`)
}

func runWorker(cmd *cobra.Command, args []string) error {
	applyRunConfigDefaults(cmd)

	// Resolve project directory
	projectDir, err := resolveProjectDir(runOpts.ProjectDir)
	if err != nil {
		return fmt.Errorf("resolving project directory: %w", err)
	}
	runOpts.ProjectDir = projectDir

	// Resolve --user into uid[:gid] for the container
	containerUser, err := resolveContainerUser(userMode, runOpts.ProjectDir)
	if err != nil {
		return fmt.Errorf("resolving --user: %w", err)
	}
	runOpts.ContainerUser = containerUser

	// Resolve approval mode: --full-auto (deprecated) maps to "full-auto"
	// when --approval-mode has not been explicitly set from its default.
	mode := sandbox.ApprovalMode(approvalModeFlag)
	if fullAutoFlag && approvalModeFlag == "suggest" {
		mode = sandbox.ApprovalModeFullAuto
	}
	if !sandbox.ValidApprovalMode(mode) {
		return fmt.Errorf("invalid --approval-mode %q; valid values: suggest, auto-edit, full-auto, danger", approvalModeFlag)
	}
	runOpts.ApprovalMode = mode

	// Resolve agent. --shell forces the plain-shell agent (no auto-launch).
	agent := sandbox.Agent(agentFlag)
	if runOpts.ShellMode {
		agent = sandbox.AgentNone
	}
	if !sandbox.ValidAgent(agent) {
		return fmt.Errorf("invalid --agent %q; valid values: codex, claude, or empty (shell)", agentFlag)
	}
	runOpts.Agent = agent

	// Network manager handles per-worker Internal networks. Each worker gets its
	// own Internal bridge shared only with the proxy (created in sandbox.Run), so
	// isolation is enforced by Docker without any iptables/sudo.
	netMgr, err := network.NewManager()
	if err != nil {
		return fmt.Errorf("creating network manager: %w", err)
	}

	proxy, err := authproxy.NewRemoteProxy(proxyAdminURL, proxyContainerURL, runProxyAdminSecret)
	if err != nil {
		// If the proxy simply isn't running and we're on a terminal with the
		// default proxy URLs, offer to build/start it now instead of failing.
		proxy, err = offerToStartProxy(cmd, err)
		if err != nil {
			return fmt.Errorf("connecting external auth proxy: %w", err)
		}
	}

	if runOpts.Agent == sandbox.AgentClaude && !proxy.IsAnthropicMode() {
		return fmt.Errorf("--agent claude requires Anthropic credentials on the auth proxy; " +
			"set ANTHROPIC_API_KEY or run `claude` OAuth login (~/.claude/.credentials.json) before starting the proxy")
	}

	// Load packages.dock if present and no --pkg-file given
	if runOpts.PkgFile == "" {
		autoFile := runOpts.ProjectDir + "/packages.dock"
		if _, err := os.Stat(autoFile); err == nil {
			runOpts.PkgFile = autoFile
		}
	}

	// Setup signal handling for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	sbMgr, err := sandbox.NewManager(sandbox.ManagerConfig{
		Proxy:        proxy,
		Network:      netMgr,
		HTTPProxyURL: httpProxyURL,
		Verbose:      verbose,
		Debug:        debug,
	})
	if err != nil {
		return fmt.Errorf("creating sandbox manager: %w", err)
	}

	// Auto-build the sandbox image when it is not present locally.
	if exists, err := sbMgr.ImageExists(runOpts.Image); err != nil {
		return fmt.Errorf("checking image %s: %w", runOpts.Image, err)
	} else if !exists {
		fmt.Printf("Image %s not found locally, building...\n", runOpts.Image)
		dockerfile, buildCtx, err := resolveDockerfile("")
		if err != nil {
			return fmt.Errorf("auto-build: %w", err)
		}
		if err := executeBuild(cmd.Context(), runOpts.Image, dockerfile, buildCtx); err != nil {
			return fmt.Errorf("auto-build: %w", err)
		}
	}

	if runOpts.Parallel > 1 {
		return runParallel(sbMgr, sigCh)
	}

	return runSingle(sbMgr, sigCh)
}

// offerToStartProxy is called when connecting to the auth proxy failed. When the
// failure looks like "proxy not running", the proxy URLs are at their defaults,
// and stdin is a terminal, it prompts the user to build/start the proxy container
// and, on yes, starts it and retries the connection. Otherwise it returns the
// original error unchanged.
func offerToStartProxy(cmd *cobra.Command, connErr error) (*authproxy.RemoteProxy, error) {
	urlsDefault := !cmd.Flags().Changed("proxy-admin-url") && !cmd.Flags().Changed("proxy-container-url")
	if !isProxyUnreachable(connErr) || !urlsDefault || !term.IsTerminal(int(os.Stdin.Fd())) {
		return nil, connErr
	}

	fmt.Println("Auth Proxy is not running.")
	if !confirmYesNo(os.Stdin, "Build/start the auth proxy container now? [y/N]: ") {
		return nil, connErr
	}

	if err := startProxyContainer(cmd.Context()); err != nil {
		return nil, fmt.Errorf("starting auth proxy: %w", err)
	}

	// The container needs a moment to bind its listeners; retry briefly.
	proxy, err := connectProxyWithRetry(proxyAdminURL, proxyContainerURL, runProxyAdminSecret, 30)
	if err != nil {
		return nil, fmt.Errorf("auth proxy started but did not become ready: %w", err)
	}
	return proxy, nil
}

// isProxyUnreachable reports whether err indicates the proxy is simply not
// listening (as opposed to, say, an auth/secret rejection that starting it
// wouldn't fix).
func isProxyUnreachable(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "connection refused") ||
		strings.Contains(s, "no such host") ||
		strings.Contains(s, "no route to host") ||
		strings.Contains(s, "i/o timeout") ||
		strings.Contains(s, "connect: ")
}

// confirmYesNo reads a line from r and returns true only for an affirmative
// answer. An empty line (just Enter) defaults to no.
func confirmYesNo(r io.Reader, prompt string) bool {
	fmt.Print(prompt)
	line, _ := bufio.NewReader(r).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}

// connectProxyWithRetry retries NewRemoteProxy a few times so a just-started
// proxy container has time to bind its listeners.
func connectProxyWithRetry(adminURL, containerURL, secret string, attempts int) (*authproxy.RemoteProxy, error) {
	var lastErr error
	for i := 0; i < attempts; i++ {
		p, err := authproxy.NewRemoteProxy(adminURL, containerURL, secret)
		if err == nil {
			return p, nil
		}
		lastErr = err
		time.Sleep(500 * time.Millisecond)
	}
	return nil, lastErr
}

func applyRunConfigDefaults(cmd *cobra.Command) {
	flags := cmd.Flags()

	if !flags.Changed("image") {
		if v := viper.GetString("run.image"); v != "" {
			runOpts.Image = v
		} else if v := viper.GetString("default_image"); v != "" {
			runOpts.Image = v
		}
	}

	if !flags.Changed("token-ttl") {
		if viper.IsSet("run.token_ttl") {
			runOpts.TokenTTL = viper.GetInt("run.token_ttl")
		} else if viper.IsSet("default_token_ttl") {
			runOpts.TokenTTL = viper.GetInt("default_token_ttl")
		}
	}

	if !flags.Changed("approval-mode") && viper.IsSet("run.approval_mode") {
		approvalModeFlag = viper.GetString("run.approval_mode")
	}

	if !flags.Changed("user") && viper.IsSet("run.user") {
		userMode = viper.GetString("run.user")
	}

	if !flags.Changed("proxy-container-url") {
		if v := viper.GetString("proxy.container_url"); v != "" {
			proxyContainerURL = v
		} else if v := viper.GetString("firewall.proxy_container_url"); v != "" {
			// Backwards compatibility with the old [firewall] config section.
			proxyContainerURL = v
		}
	}
}

func runSingle(mgr *sandbox.Manager, sigCh <-chan os.Signal) error {
	opts := runOpts

	// Handle worktree
	if opts.UseWorktree {
		wtPath, err := worktree.Create(worktree.CreateOptions{
			ProjectDir: opts.ProjectDir,
			Branch:     opts.Branch,
			NewBranch:  opts.NewBranch,
		})
		if err != nil {
			return fmt.Errorf("creating worktree: %w", err)
		}
		opts.WorktreePath = wtPath
		defer func() {
			if err := worktree.Remove(wtPath); err != nil && verbose {
				fmt.Fprintf(os.Stderr, "warning: removing worktree %s: %v\n", wtPath, err)
			}
		}()
	}

	containerID, err := mgr.Run(opts)
	if err != nil {
		return fmt.Errorf("starting container: %w", err)
	}

	if opts.Detach {
		fmt.Printf("Container started: %s\n", containerID[:12])
		return nil
	}

	// Wait for signal or container exit
	exitCh := mgr.Wait(containerID)
	var runErr error
	select {
	case <-sigCh:
		fmt.Println("\nStopping container...")
		runErr = mgr.Stop(containerID, 10) // Stop() revokes token (F-AUTH-04)
	case err := <-exitCh:
		// Container exited on its own — revoke its token (F-AUTH-04)
		mgr.RevokeToken(containerID)
		runErr = err
	}

	// Foreground cleanup: remove the container and its per-worker Internal network
	// so they don't accumulate across runs (use --keep to retain them).
	if !runKeep {
		if err := mgr.Remove(containerID, true); err != nil && verbose {
			fmt.Fprintf(os.Stderr, "warning: cleaning up container/network: %v\n", err)
		}
	}
	return runErr
}

func runParallel(mgr *sandbox.Manager, sigCh <-chan os.Signal) error {
	opts := runOpts
	n := opts.Parallel
	fmt.Printf("Starting %d parallel workers...\n", n)

	containerIDs := make([]string, 0, n)
	worktreePaths := make([]string, 0, n)

	for i := 1; i <= n; i++ {
		o := opts
		branch := opts.Branch
		if branch == "" {
			branch = fmt.Sprintf("worker-%d", i)
		} else {
			branch = fmt.Sprintf("%s-%d", branch, i)
		}
		o.Branch = branch
		o.Name = fmt.Sprintf("%s-%d", opts.Name, i)

		if opts.UseWorktree {
			wtPath, err := worktree.Create(worktree.CreateOptions{
				ProjectDir: opts.ProjectDir,
				Branch:     branch,
				NewBranch:  true,
			})
			if err != nil {
				// cleanup already-created worktrees
				for _, p := range worktreePaths {
					_ = worktree.Remove(p)
				}
				return fmt.Errorf("creating worktree %d: %w", i, err)
			}
			worktreePaths = append(worktreePaths, wtPath)
			o.WorktreePath = wtPath
		}

		containerID, err := mgr.Run(o)
		if err != nil {
			for _, p := range worktreePaths {
				_ = worktree.Remove(p)
			}
			return fmt.Errorf("starting worker %d: %w", i, err)
		}
		containerIDs = append(containerIDs, containerID)
		fmt.Printf("  Worker %d started: %s\n", i, containerID[:12])
	}

	if opts.Detach {
		return nil
	}

	<-sigCh
	fmt.Println("\nStopping all workers...")
	for _, id := range containerIDs {
		if err := mgr.Stop(id, 10); err != nil && verbose {
			fmt.Fprintf(os.Stderr, "warning: stopping %s: %v\n", id[:12], err)
		}
		// Remove the container and its per-worker network so they don't accumulate.
		if !runKeep {
			if err := mgr.Remove(id, true); err != nil && verbose {
				fmt.Fprintf(os.Stderr, "warning: cleaning up %s: %v\n", id[:12], err)
			}
		}
	}
	for _, p := range worktreePaths {
		if err := worktree.Remove(p); err != nil && verbose {
			fmt.Fprintf(os.Stderr, "warning: removing worktree %s: %v\n", p, err)
		}
	}
	return nil
}

func resolveProjectDir(dir string) (string, error) {
	if dir == "." {
		return os.Getwd()
	}
	return dir, nil
}

// resolveContainerUser converts a --user mode string into a "uid:gid" value
// suitable for container.Config.User.
//
//	""        → "" (use image default)
//	"current" → "<uid>:<gid>" of the process running this command
//	"codex"   → "1001:1001" (codex user used by the default image)
//	"dir"     → "<uid>:<gid>" of the owner of projectDir
//	anything else is returned as-is (e.g. "1000", "1000:1000")
func resolveContainerUser(mode, projectDir string) (string, error) {
	switch mode {
	case "":
		return "", nil
	case "current":
		uid := syscall.Getuid()
		gid := syscall.Getgid()
		return fmt.Sprintf("%d:%d", uid, gid), nil
	case "codex":
		return "1001:1001", nil
	case "dir":
		info, err := os.Stat(projectDir)
		if err != nil {
			return "", fmt.Errorf("stat %s: %w", projectDir, err)
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return "", fmt.Errorf("cannot read uid/gid from %s on this platform", projectDir)
		}
		return fmt.Sprintf("%d:%d", stat.Uid, stat.Gid), nil
	default:
		return mode, nil
	}
}
