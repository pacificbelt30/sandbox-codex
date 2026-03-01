package sandbox

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/pacificbelt30/codex-dock/internal/authproxy"
	"github.com/pacificbelt30/codex-dock/internal/network"
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

	isOAuth := authproxy.IsOAuthAuth()

	if m.proxy != nil && !isOAuth {
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
	if installScript != "" {
		env = append(env, "CODEX_INSTALL_SCRIPT="+installScript)
	}

	// Mount configuration
	mountMode := "rw"
	if opts.ReadOnly {
		mountMode = "ro"
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
	_ = mountMode // applied via ReadOnly field below

	// OAuth (ChatGPT subscription) auth: mount auth.json directly into the
	// container so the Codex CLI can read and refresh tokens as usual.
	if isOAuth {
		authJSONPath, err := authproxy.CodexAuthJSONPath()
		if err != nil {
			return "", fmt.Errorf("resolving auth.json path: %w", err)
		}
		mounts = append(mounts, mount.Mount{
			Type:   mount.TypeBind,
			Source: authJSONPath,
			Target: "/home/codex/.codex/auth.json",
			BindOptions: &mount.BindOptions{
				Propagation: mount.PropagationRPrivate,
			},
		})
		if m.debug {
			fmt.Fprintf(os.Stderr, "debug: mounting auth.json for OAuth auth\n")
		}
	}

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
		Cmd:          []string{"/entrypoint.sh"},
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

// Stop gracefully stops a container by ID.
func (m *Manager) Stop(containerID string, timeoutSec int) error {
	ctx := context.Background()
	return m.cli.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeoutSec})
}

// StopByName stops a container by name.
func (m *Manager) StopByName(name string, timeoutSec int) error {
	id, err := m.resolveID(name)
	if err != nil {
		return err
	}
	return m.Stop(id, timeoutSec)
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

// Logs streams container logs.
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

	_, err = io.Copy(os.Stdout, rc)
	return err
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

	go func() { _, _ = io.Copy(resp.Conn, os.Stdin) }()
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
