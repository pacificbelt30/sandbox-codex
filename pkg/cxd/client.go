// Package cxd exposes a local Go API for products embedding codex-dock workers.
// It talks to the existing root-owned node Unix socket; it does not start a
// listener or grant access beyond the caller's node-socket permissions.
package cxd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"path/filepath"
	"time"

	"github.com/pacificbelt30/codex-dock/internal/authproxy"
	"github.com/pacificbelt30/codex-dock/internal/engine"
	"github.com/pacificbelt30/codex-dock/internal/kataguard"
	"github.com/pacificbelt30/codex-dock/internal/network"
	"github.com/pacificbelt30/codex-dock/internal/networkpolicy"
	"github.com/pacificbelt30/codex-dock/internal/sandbox"
	"github.com/pacificbelt30/codex-dock/internal/workerapp"
	"github.com/pacificbelt30/codex-dock/internal/workeregress"
	"github.com/pacificbelt30/codex-dock/internal/workerpolicy"
	"github.com/pacificbelt30/codex-dock/pkg/cxd/identity"
)

// Options contains trusted host settings. A cloned project cannot set these.
type Options struct {
	NodeSocket        string
	ProxyAdminURL     string
	ProxyContainerURL string
	ProxyAdminSecret  string
	HTTPProxyURL      string
	EgressProxyImage  string
	EgressPolicy      EgressPolicy
	StateDir          string
}

// Client is a handle to the local node. Close it when the product shuts down.
type Client struct {
	manager *sandbox.Manager
	node    *kataguard.Client
	opts    Options
}

// Open connects to the node without starting an auth proxy or a worker.
func Open(ctx context.Context, opts Options) (*Client, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if opts.NodeSocket == "" {
		opts.NodeSocket = engine.Socket()
	}
	if opts.ProxyAdminURL == "" {
		opts.ProxyAdminURL = "http://127.0.0.1:18081"
	}
	if opts.ProxyContainerURL == "" {
		opts.ProxyContainerURL = "http://codex-auth-proxy:18080"
	}
	if opts.HTTPProxyURL == "" {
		opts.HTTPProxyURL = "http://codex-http-proxy:18082"
	}
	mgr, err := sandbox.NewManager(sandbox.ManagerConfig{
		Socket: opts.NodeSocket, KataConfig: &kataguard.Config{Socket: opts.NodeSocket}, StateDir: opts.StateDir,
		ProxyConnector: func(adminURL, containerURL string) (authproxy.Service, error) {
			if adminURL == "" {
				adminURL = opts.ProxyAdminURL
			}
			if containerURL == "" {
				containerURL = opts.ProxyContainerURL
			}
			return authproxy.NewRemoteProxy(adminURL, containerURL, opts.ProxyAdminSecret)
		},
	})
	if err != nil {
		return nil, err
	}
	if _, err := mgr.ListContext(ctx, true); err != nil {
		_ = mgr.Close()
		return nil, fmt.Errorf("connecting to cxd node: %w", err)
	}
	node, err := kataguard.Connect(opts.NodeSocket, "unix://"+opts.NodeSocket)
	if err != nil {
		_ = mgr.Close()
		return nil, err
	}
	if err := node.Validate(ctx, sandbox.DefaultKataRuntime); err != nil {
		_ = node.Close()
		_ = mgr.Close()
		return nil, fmt.Errorf("validating cxd node: %w", classify(err))
	}
	if node.ProtocolVersion() != 2 {
		_ = node.Close()
		_ = mgr.Close()
		return nil, fmt.Errorf("%w: node does not support Go client protocol v2", ErrVersionMismatch)
	}
	return &Client{manager: mgr, node: node, opts: opts}, nil
}

func (c *Client) Close() error { return errors.Join(c.manager.Close(), c.node.Close()) }

// LaunchRequest describes one worker. An image must already be built or
// imported into the dedicated node. Agent may be "codex" or "claude"; when
// empty the worker stays idle so the product can call Exec repeatedly.
type FilesystemPolicy = workerpolicy.Filesystem
type LifecyclePolicy = workerpolicy.Lifecycle
type EnvironmentPolicy = workerpolicy.Environment

type Compatibility = sandbox.Compatibility

type LaunchRequest struct {
	Compatibility Compatibility     `json:",omitzero"`
	Filesystem    FilesystemPolicy  `json:",omitzero"`
	Lifecycle     LifecyclePolicy   `json:",omitzero"`
	Environment   EnvironmentPolicy `json:",omitzero"`
	CloneDir      string            `json:",omitempty"`
	// RequestID is a stable, node-wide identifier for this launch attempt.
	// Reuse it only with exactly the same immutable options.
	RequestID          string
	ProjectDir         string
	Image              string
	Profile            string
	Runtime            string
	Agent              string
	AgentArgs          []string
	Model              string
	ApprovalMode       string
	Branch             string
	UseWorktree        bool
	NewBranch          bool
	ReadOnly           bool
	ReadOnlyRootFS     bool
	NoGitMetadata      bool
	NoInternet         bool
	PolicyPreset       string `json:",omitempty"`
	EgressAllowDomains []string
	WatchPolling       bool
	User               string
	Mounts             []Mount
	TokenTTL           time.Duration
	Identity           identity.Identity
}

// Mount is an additional host bind mount. Node socket access already grants
// equivalent host authority, so only trusted products may provide mounts.
type Mount struct {
	Source   string
	Target   string
	ReadOnly bool
}

// Worker describes a managed worker without exposing Docker/containerd types.
type Worker struct {
	ID         string
	Name       string
	Status     string
	Image      string
	Branch     string
	StartedAt  *time.Time
	FinishedAt *time.Time
}

func fromWorker(w sandbox.Worker) Worker {
	return Worker{ID: w.ID, Name: w.Name, Status: w.Status, Image: w.Image, Branch: w.Branch, StartedAt: w.StartedAt, FinishedAt: w.FinishedAt}
}

// Launch creates and starts a detached worker. The returned ID remains useful
// if cleanup fails after a canceled launch, so callers can retry Remove.
func (c *Client) Launch(ctx context.Context, req LaunchRequest) (Worker, error) {
	if err := req.Filesystem.Validate(); err != nil {
		return Worker{}, err
	}
	if err := req.Lifecycle.Validate(); err != nil {
		return Worker{}, err
	}
	if err := req.Environment.Validate(); err != nil {
		return Worker{}, err
	}
	key, err := requestKey(req.RequestID)
	if err != nil {
		return Worker{}, err
	}
	if req.ProjectDir == "" || !filepath.IsAbs(req.ProjectDir) {
		return Worker{}, fmt.Errorf("project directory must be an absolute path")
	}
	if !sandbox.ValidAgent(sandbox.Agent(req.Agent)) {
		return Worker{}, fmt.Errorf("invalid agent %q", req.Agent)
	}
	if req.ApprovalMode != "" && !sandbox.ValidApprovalMode(sandbox.ApprovalMode(req.ApprovalMode)) {
		return Worker{}, fmt.Errorf("invalid approval mode %q", req.ApprovalMode)
	}
	if req.TokenTTL < 0 || req.TokenTTL > time.Duration(math.MaxInt32)*time.Second {
		return Worker{}, fmt.Errorf("invalid token TTL")
	}
	if !req.Identity.Empty() {
		if err := req.Identity.Validate(); err != nil {
			return Worker{}, err
		}
		if req.Agent == "codex" && req.Identity.Provider != "openai" || req.Agent == "claude" && req.Identity.Provider != "anthropic" {
			return Worker{}, fmt.Errorf("token provider does not match selected agent")
		}
	}
	if req.TokenTTL == 0 {
		req.TokenTTL = time.Hour
	}
	if req.Image == "" {
		req.Image = "codex-dock:latest"
	}
	if req.Profile == "" {
		req.Profile = "standard"
	}
	if req.Runtime == "" {
		req.Runtime = sandbox.RuntimeKata
	}
	resolved, offline, err := workeregress.ResolvePreset(c.opts.EgressPolicy.rules(), req.PolicyPreset, req.EgressAllowDomains)
	if err != nil {
		return Worker{}, err
	}
	if offline {
		req.NoInternet = true
	}
	if req.PolicyPreset != "" && !offline {
		req.EgressAllowDomains = resolved.AllowDomains
	}
	if req.NoInternet && len(req.EgressAllowDomains) != 0 {
		return Worker{}, fmt.Errorf("egress allowlist cannot be combined with no-internet")
	}
	var egressRules networkpolicy.Rules
	var egressPolicyID string
	if len(req.EgressAllowDomains) != 0 {
		egressRules, err = workeregress.Rules(c.opts.EgressPolicy.rules(), req.EgressAllowDomains)
		if err != nil {
			return Worker{}, err
		}
		egressPolicyID = workeregress.ID(egressRules)
	}
	autoBranch := req.UseWorktree && req.Branch == ""
	if autoBranch {
		req.Branch = "cxd-" + key[:16]
		req.NewBranch = true
	}
	// Proxy location affects the resulting worker but the admin secret is never
	// persisted or hashed into the node's request ledger.
	fingerprintInput, err := json.Marshal(struct {
		LaunchRequest
		ProxyContainerURL string
		ProxyAdminURL     string
		HTTPProxyURL      string
		EgressProxyImage  string
		EgressPolicyID    string
	}{req, c.opts.ProxyContainerURL, c.opts.ProxyAdminURL, c.opts.HTTPProxyURL, c.opts.EgressProxyImage, egressPolicyID})
	if err != nil {
		return Worker{}, err
	}
	fingerprintBytes := sha256.Sum256(fingerprintInput)
	fingerprint := hex.EncodeToString(fingerprintBytes[:])
	claim, err := c.node.ClaimLaunch(ctx, key, fingerprint)
	if err != nil {
		return Worker{}, classify(err)
	}
	if claim.State == "complete" {
		w, err := c.Get(ctx, claim.WorkerID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return Worker{ID: claim.WorkerID, Name: claim.Name}, fmt.Errorf("%w: %s", ErrRequestGone, claim.WorkerID)
			}
			return Worker{ID: claim.WorkerID, Name: claim.Name}, err
		}
		return w, nil
	}
	if claim.State == "cleanup-pending" {
		return Worker{ID: claim.WorkerID, Name: claim.Name}, fmt.Errorf("%w for worker %s", ErrCleanupPending, claim.WorkerID)
	}
	if claim.State != "pending" || claim.Lease == "" {
		return Worker{Name: claim.Name}, fmt.Errorf("%w: request %s is active or needs recovery", ErrInProgress, req.RequestID)
	}
	httpProxyURL := c.opts.HTTPProxyURL
	if len(req.EgressAllowDomains) != 0 {
		var policyErr error
		httpProxyURL, policyErr = workeregress.Ensure(ctx, c.opts.NodeSocket, c.opts.EgressProxyImage, egressRules)
		if policyErr != nil {
			return Worker{}, errors.Join(policyErr, c.abortClaim(key, claim.Lease, ""))
		}
	}
	proxy, err := authproxy.NewRemoteProxyContext(ctx, c.opts.ProxyAdminURL, c.opts.ProxyContainerURL, c.opts.ProxyAdminSecret)
	if err != nil {
		abortErr := c.abortClaim(key, claim.Lease, "")
		return Worker{}, errors.Join(fmt.Errorf("connecting auth proxy: %w", err), abortErr)
	}
	if proxy.IsCaseProxy() && req.Identity.Empty() {
		return Worker{}, errors.Join(fmt.Errorf("case proxy requires a token identity"), c.abortClaim(key, claim.Lease, ""))
	}
	netMgr, err := network.NewManagerAt(c.opts.NodeSocket)
	if err != nil {
		return Worker{}, errors.Join(err, c.abortClaim(key, claim.Lease, ""))
	}
	defer func() { _ = netMgr.Close() }()
	mgr, err := sandbox.NewManager(sandbox.ManagerConfig{
		Socket: c.opts.NodeSocket, Proxy: proxy, Network: netMgr, HTTPProxyURL: httpProxyURL,
		ProxyAdminURL: c.opts.ProxyAdminURL, StateDir: c.opts.StateDir,
		KataConfig: &kataguard.Config{Socket: c.opts.NodeSocket},
	})
	if err != nil {
		return Worker{}, errors.Join(err, c.abortClaim(key, claim.Lease, ""))
	}
	defer func() { _ = mgr.Close() }()
	launch := sandbox.RunOptions{
		Compatibility: req.Compatibility, Filesystem: req.Filesystem, Lifecycle: req.Lifecycle, Environment: req.Environment, CloneDir: req.CloneDir,
		ProjectDir: req.ProjectDir, Image: req.Image, ResourceProfile: req.Profile, Runtime: req.Runtime,
		Name: claim.Name, RequestKey: key, RequestFingerprint: fingerprint, AllocationID: kataguard.RequestAllocation(key),
		Agent: sandbox.Agent(req.Agent), DirectAgent: req.Agent != "", AgentArgs: req.AgentArgs,
		Model: req.Model, ApprovalMode: sandbox.ApprovalMode(req.ApprovalMode),
		Branch: req.Branch, UseWorktree: req.UseWorktree, NewBranch: req.NewBranch, ReuseBranchOnRetry: autoBranch && claim.Attempt > 1,
		ReadOnly: req.ReadOnly, ReadOnlyRootFS: req.ReadOnlyRootFS, NoGitMetadata: req.NoGitMetadata,
		NoInternet: req.NoInternet, WatchPolling: req.WatchPolling,
		ContainerUser: req.User, TokenTTL: int(req.TokenTTL / time.Second), Detach: true,
		TokenIdentity: req.Identity,
	}
	for _, mount := range req.Mounts {
		launch.Mounts = append(launch.Mounts, sandbox.MountSpec{Source: mount.Source, Target: mount.Target, ReadOnly: mount.ReadOnly})
	}
	if req.Agent == "" {
		launch.ExecCmd = "exec sleep infinity"
	}
	id, err := workerapp.Launch(ctx, mgr, launch)
	if err != nil {
		abortErr := c.abortClaim(key, claim.Lease, id)
		return Worker{ID: id, Name: claim.Name}, errors.Join(classify(err), abortErr)
	}
	finishCtx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	_, finishErr := c.node.FinishLaunch(finishCtx, key, claim.Lease, id)
	cancel()
	if finishErr != nil {
		return Worker{ID: id, Name: claim.Name}, fmt.Errorf("%w: worker started but launch confirmation failed: %w", ErrOutcomeUnknown, finishErr)
	}
	w, err := mgr.InspectContext(ctx, id)
	if err != nil {
		return Worker{ID: id, Name: claim.Name}, fmt.Errorf("%w: worker started; inspect %s: %w", ErrOutcomeUnknown, id, err)
	}
	return fromWorker(w), nil
}

func (c *Client) abortClaim(key, lease, id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return c.node.AbortLaunch(ctx, key, lease, id)
}

func (c *Client) Get(ctx context.Context, id string) (Worker, error) {
	w, err := c.manager.InspectContext(ctx, id)
	return fromWorker(w), classify(err)
}

func (c *Client) List(ctx context.Context) ([]Worker, error) {
	workers, err := c.manager.ListContext(ctx, true)
	if err != nil {
		return nil, classify(err)
	}
	result := make([]Worker, 0, len(workers))
	for _, w := range workers {
		result = append(result, fromWorker(w))
	}
	return result, nil
}

// ExecRequest streams a command directly to and from an existing worker.
type ExecRequest struct {
	WorkingDir string
	Env        []string
	TTY        bool
	Argv       []string
	Stdin      io.Reader
	Stdout     io.Writer
	Stderr     io.Writer
}

// Exec returns the guest exit code separately from transport errors.
func (c *Client) Exec(ctx context.Context, id string, req ExecRequest) (int64, error) {
	code, err := c.manager.ExecWithOptions(ctx, id, sandbox.ExecOptions{Argv: req.Argv, Stdin: req.Stdin, Stdout: req.Stdout, Stderr: req.Stderr, WorkingDir: req.WorkingDir, Env: req.Env, TTY: req.TTY})
	return code, classify(err)
}

// Wait waits for the main worker process. Canceling ctx only cancels the wait.
func (c *Client) Wait(ctx context.Context, id string) (int64, error) {
	code, err := c.manager.WaitContext(ctx, id)
	return code, classify(err)
}

func (c *Client) Start(ctx context.Context, id string) error {
	return classify(c.manager.StartContext(ctx, id))
}

func (c *Client) Stop(ctx context.Context, id string, timeout time.Duration) error {
	if timeout < 0 || timeout > time.Duration(math.MaxInt32)*time.Second {
		return fmt.Errorf("invalid stop timeout")
	}
	return classify(c.manager.StopContext(ctx, id, int(timeout/time.Second)))
}

func (c *Client) Remove(ctx context.Context, id string, force bool) error {
	return classify(c.manager.RemoveContext(ctx, id, force))
}

func (c *Client) RevokeRun(ctx context.Context, id identity.Identity) error {
	proxy, err := authproxy.NewRemoteProxyContext(ctx, c.opts.ProxyAdminURL, c.opts.ProxyContainerURL, c.opts.ProxyAdminSecret)
	if err != nil {
		return err
	}
	return proxy.RevokeRun(ctx, id)
}

func (c *Client) RevokeCase(ctx context.Context) error {
	proxy, err := authproxy.NewRemoteProxyContext(ctx, c.opts.ProxyAdminURL, c.opts.ProxyContainerURL, c.opts.ProxyAdminSecret)
	if err != nil {
		return err
	}
	return proxy.RevokeCase(ctx)
}

// Profile reports the node's pinned reservation contract, not measured RSS.
type Profile struct {
	Name             string
	CPUCores         int
	GuestMemoryBytes int64
	HostMemoryBytes  int64
	PIDLimit         int64
}

func Profiles() []Profile {
	profiles := kataguard.ResourceProfiles()
	result := make([]Profile, 0, len(profiles))
	for _, p := range profiles {
		result = append(result, Profile{p.Name, p.CPUCores, p.GuestMemoryBytes, p.HostMemoryBytes, p.PIDLimit})
	}
	return result
}

// Profiles queries the running node's supported resource reservations.
func (c *Client) Profiles(ctx context.Context) ([]Profile, error) {
	profiles, err := c.node.Profiles(ctx)
	if err != nil {
		return nil, classify(err)
	}
	result := make([]Profile, 0, len(profiles))
	for _, p := range profiles {
		result = append(result, Profile{p.Name, p.CPUCores, p.GuestMemoryBytes, p.HostMemoryBytes, p.PIDLimit})
	}
	return result, nil
}
