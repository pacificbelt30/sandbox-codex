package cmd

import (
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/pacificbelt30/authproxy"
	"github.com/pacificbelt30/codex-dock/internal/network"
	"github.com/pacificbelt30/codex-dock/internal/sandbox"
	"github.com/pacificbelt30/codex-dock/internal/worktree"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// userMode is the raw value of --user before resolution.
var userMode string

// approvalModeFlag holds the raw value of --approval-mode before validation.
var approvalModeFlag string

// fullAutoFlag is a deprecated alias for --approval-mode full-auto.
var fullAutoFlag bool

var runOpts sandbox.RunOptions
var (
	proxyAdminURL       string
	proxyContainerURL   string
	runProxyAdminSecret string
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
	f.StringVarP(&runOpts.Task, "task", "t", "", "Initial task prompt for Codex")
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
	f.BoolVar(&runOpts.NoInternet, "no-internet", false, "Disable internet access inside container")
	f.IntVar(&runOpts.TokenTTL, "token-ttl", 3600, "Token TTL in seconds")
	f.StringVar(&runOpts.AgentsMD, "agents-md", "", "Path to additional AGENTS.md")
	f.StringVar(&proxyAdminURL, "proxy-admin-url", "http://127.0.0.1:18080", "External auth proxy admin URL")
	f.StringVar(&proxyContainerURL, "proxy-container-url", "http://codex-auth-proxy:18080", "Auth proxy URL reachable from worker containers")
	f.StringVar(&runProxyAdminSecret, "proxy-admin-secret", "", "Admin secret for external auth proxy")
	f.BoolVarP(&runOpts.Detach, "detach", "D", false, "Run container in background")
	f.IntVarP(&runOpts.Parallel, "parallel", "P", 1, "Number of parallel workers")
	f.BoolVarP(&runOpts.ShellMode, "shell", "s", false, "Start an interactive bash shell instead of Codex")
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

	// Ensure dock-net exists
	netMgr, err := network.NewManager()
	if err != nil {
		return fmt.Errorf("creating network manager: %w", err)
	}
	ensureOpts := network.EnsureOptions{
		NoInternet: runOpts.NoInternet,
	}
	if port, ok := allowedHostPort(proxyContainerURL); ok {
		ensureOpts.AllowHostTCPPorts = []int{port}
	}
	if endpoint, ok := network.AllowHostEndpoint(proxyContainerURL); ok {
		ensureOpts.AllowTCPDestinations = []network.HostEndpoint{endpoint}
	}
	if err := netMgr.EnsureNetwork(ensureOpts); err != nil {
		return fmt.Errorf("ensuring dock-net: %w", err)
	}
	if err := netMgr.ApplyFirewall(ensureOpts); err != nil {
		if network.IsFirewallWarning(err) {
			fmt.Printf("Warning: dock-net firewall rules were not applied: %v\n", err)
		} else {
			return fmt.Errorf("ensuring dock-net firewall: %w", err)
		}
	}

	proxy, err := authproxy.NewRemoteProxy(proxyAdminURL, proxyContainerURL, runProxyAdminSecret)
	if err != nil {
		return fmt.Errorf("connecting external auth proxy: %w", err)
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
		Proxy:   proxy,
		Network: netMgr,
		Verbose: verbose,
		Debug:   debug,
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
}

func allowedHostPort(rawURL string) (int, bool) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Port() == "" {
		return 0, false
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port <= 0 || port > 65535 {
		return 0, false
	}
	return port, true
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
	select {
	case <-sigCh:
		fmt.Println("\nStopping container...")
		return mgr.Stop(containerID, 10) // Stop() revokes token (F-AUTH-04)
	case err := <-exitCh:
		// Container exited on its own — revoke its token (F-AUTH-04)
		mgr.RevokeToken(containerID)
		return err
	}
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
