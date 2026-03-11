package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/pacificbelt30/codex-dock/internal/authproxy"
	"github.com/pacificbelt30/codex-dock/internal/network"
	"github.com/pacificbelt30/codex-dock/internal/sandbox"
	"github.com/pacificbelt30/codex-dock/internal/worktree"
	"github.com/spf13/cobra"
)

// userMode is the raw value of --user before resolution.
var userMode string

var runOpts sandbox.RunOptions

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
	f.BoolVar(&runOpts.FullAuto, "full-auto", false, "Run Codex with --ask-for-approval never")
	f.StringVarP(&runOpts.Model, "model", "m", "", "Model name to pass to Codex")
	f.BoolVar(&runOpts.ReadOnly, "read-only", false, "Mount project as read-only")
	f.BoolVar(&runOpts.NoInternet, "no-internet", false, "Disable internet access inside container")
	f.IntVar(&runOpts.TokenTTL, "token-ttl", 3600, "Token TTL in seconds")
	f.StringVar(&runOpts.AgentsMD, "agents-md", "", "Path to additional AGENTS.md")
	f.BoolVarP(&runOpts.Detach, "detach", "D", false, "Run container in background")
	f.IntVarP(&runOpts.Parallel, "parallel", "P", 1, "Number of parallel workers")
	f.BoolVarP(&runOpts.ShellMode, "shell", "s", false, "Start an interactive bash shell instead of Codex")
	f.StringVar(&userMode, "user", "", `User to run as inside the container.
  ""        image default (uid:1001 codex user)
  "current" current command user (uid:gid)
  "dir"     project directory owner (uid:gid)
  "uid"     explicit uid (e.g. "1000" or "1000:1000")`)
}

func runWorker(cmd *cobra.Command, args []string) error {
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

	// Ensure dock-net exists
	netMgr, err := network.NewManager()
	if err != nil {
		return fmt.Errorf("creating network manager: %w", err)
	}
	if err := netMgr.EnsureNetwork(runOpts.NoInternet); err != nil {
		return fmt.Errorf("ensuring dock-net: %w", err)
	}

	// Determine Auth Proxy listen address.
	// Use the dock-net gateway so containers can reach the proxy (F-NET-04).
	// Fall back to loopback when Docker is unavailable or the network is missing.
	listenAddr := ""
	if gwAddr, err := netMgr.GatewayAddr(); err == nil {
		listenAddr = gwAddr + ":0"
	}

	// Start Auth Proxy
	proxy, err := authproxy.NewProxy(authproxy.Config{
		TokenTTL:   runOpts.TokenTTL,
		Verbose:    verbose,
		ListenAddr: listenAddr,
	})
	if err != nil {
		return fmt.Errorf("starting auth proxy: %w", err)
	}
	if err := proxy.Start(); err != nil {
		return fmt.Errorf("starting auth proxy: %w", err)
	}
	defer proxy.Stop()

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
		if err := executeBuild(runOpts.Image, dockerfile, buildCtx); err != nil {
			return fmt.Errorf("auto-build: %w", err)
		}
	}

	if runOpts.Parallel > 1 {
		return runParallel(sbMgr, proxy, sigCh)
	}

	return runSingle(sbMgr, proxy, sigCh)
}

func runSingle(mgr *sandbox.Manager, proxy *authproxy.Proxy, sigCh <-chan os.Signal) error {
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

func runParallel(mgr *sandbox.Manager, proxy *authproxy.Proxy, sigCh <-chan os.Signal) error {
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
