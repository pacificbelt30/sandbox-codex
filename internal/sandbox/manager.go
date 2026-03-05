package sandbox

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/pacificbelt30/codex-dock/internal/authproxy"
	"github.com/pacificbelt30/codex-dock/internal/network"
	"golang.org/x/term"
)

const (
	labelPrefix    = "codex-dock."
	labelManaged   = labelPrefix + "managed"
	labelBranch    = labelPrefix + "branch"
	labelTask      = labelPrefix + "task"
	sandboxNetName = "dock-net"
)

// ManagerConfig holds configuration for the sandbox manager.
type ManagerConfig struct {
	Proxy   *authproxy.Proxy
	Network *network.Manager
	Verbose bool
	Debug   bool
}

// Manager handles container lifecycle for codex-dock workers.
type Manager struct {
	cli     *client.Client
	proxy   *authproxy.Proxy
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
		name = generateName()
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

	// Build environment variables
	env := []string{
		"CODEX_SANDBOX=1",
	}

	if m.proxy != nil {
		token, err := m.proxy.IssueToken(name, opts.TokenTTL)
		if err != nil {
			return "", fmt.Errorf("issuing auth token: %w", err)
		}
		env = append(env,
			"CODEX_AUTH_PROXY_URL="+m.proxy.Endpoint(),
			"CODEX_TOKEN="+token,
		)
		if m.debug {
			fmt.Fprintf(os.Stderr, "debug: issued token for %s (TTL=%ds)\n", name, opts.TokenTTL)
		}
	}

	if opts.Task != "" {
		env = append(env, "CODEX_TASK="+opts.Task)
	}
	if opts.Model != "" {
		env = append(env, "CODEX_MODEL="+opts.Model)
	}
	if opts.FullAuto {
		env = append(env, "CODEX_FULL_AUTO=1")
	}
	if opts.AgentsMD != "" {
		env = append(env, "CODEX_AGENTS_MD="+opts.AgentsMD)
	}
	if installScript != "" {
		env = append(env, "CODEX_INSTALL_SCRIPT="+installScript)
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

	// Security: drop all capabilities, non-root user
	hostConfig := &container.HostConfig{
		NetworkMode: container.NetworkMode(sandboxNetName),
		Mounts:      mounts,
		CapDrop:     []string{"ALL"},
		SecurityOpt: []string{"no-new-privileges:true"},
		Resources: container.Resources{
			PidsLimit: int64ptr(512),
		},
	}

	if opts.ReadOnly {
		hostConfig.Mounts[0].ReadOnly = true
	}

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

// Remove removes a container by ID.
func (m *Manager) Remove(containerID string, force bool) error {
	return m.cli.ContainerRemove(context.Background(), containerID, container.RemoveOptions{Force: force})
}

// RemoveByName removes a container by name.
func (m *Manager) RemoveByName(name string, force bool) error {
	id, err := m.resolveID(name)
	if err != nil {
		return err
	}
	return m.Remove(id, force)
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
	defer rc.Close()

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
	// savedState holds the most recent raw-mode terminal state so it can be
	// restored before suspending and re-acquired on resume.
	var savedState *term.State
	if term.IsTerminal(fd) {
		st, err := term.MakeRaw(fd)
		if err == nil {
			savedState = st
			defer func() {
				if savedState != nil {
					_ = term.Restore(fd, savedState)
				}
			}()
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

	// Forward stdin to the container, intercepting Ctrl+Z (byte 0x1a) for job
	// control.  In raw mode the terminal driver does not convert Ctrl+Z to
	// SIGTSTP, so we detect the byte ourselves: restore the terminal, raise
	// SIGTSTP to suspend the process, then re-enter raw mode when resumed
	// by the shell's "fg" command (SIGCONT).
	go func() {
		buf := make([]byte, 32)
		for {
			n, readErr := os.Stdin.Read(buf)
			if n > 0 {
				data := buf[:n]
				if savedState != nil {
					hasSuspend := false
					for _, b := range data {
						if b == 0x1a { // Ctrl+Z
							hasSuspend = true
							break
						}
					}
					if hasSuspend {
						_ = term.Restore(fd, savedState)
						savedState = nil
						_ = syscall.Kill(syscall.Getpid(), syscall.SIGTSTP)
						// Execution resumes here after SIGCONT (fg)
						st, rawErr := term.MakeRaw(fd)
						if rawErr == nil {
							savedState = st
						}
						continue
					}
				}
				if _, werr := resp.Conn.Write(data); werr != nil {
					return
				}
			}
			if readErr != nil {
				return
			}
		}
	}()
	_, _ = io.Copy(os.Stdout, resp.Reader)
}

func buildCodexArgs(opts RunOptions) []string {
	args := []string{"codex"}
	if opts.FullAuto {
		args = append(args, "--ask-for-approval", "never")
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

func int64ptr(i int64) *int64 { return &i }
