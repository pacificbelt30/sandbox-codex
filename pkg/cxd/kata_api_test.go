package cxd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/pacificbelt30/codex-dock/internal/engine"
	"github.com/pacificbelt30/codex-dock/pkg/cxd/credentialstore"
	"github.com/pacificbelt30/codex-dock/pkg/cxd/identity"
)

type apiFixtureStore struct {
	mu           sync.Mutex
	creds        credentialstore.Credentials
	mode         string
	loads, saves int
}

func (s *apiFixtureStore) Load(context.Context, string) (credentialstore.Credentials, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loads++
	return s.creds, nil
}
func (s *apiFixtureStore) SaveRefresh(_ context.Context, _ string, version, provider string, next credentialstore.Credentials) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saves++
	if s.mode == "save-failure" {
		return "", fmt.Errorf("synthetic storage failure")
	}
	if s.mode == "conflict" {
		s.creds.Version = "concurrent-version"
	}
	if version != s.creds.Version {
		return "", credentialstore.ErrConflict
	}
	if provider != "openai" {
		return "", fmt.Errorf("unexpected provider")
	}
	next.Version = version + "-next"
	s.creds = next
	return next.Version, nil
}

// TestKataAPIIntegration drives the public Go API against the real dedicated
// node, VM, proxy container and Unix credential adapter. The authproxy test
// executable supplies synthetic upstreams; no external AI or real key is used.
func TestKataAPIIntegration(t *testing.T) {
	if os.Getenv("CODEX_DOCK_KATA_API_E2E") != "1" {
		t.Skip("set CODEX_DOCK_KATA_API_E2E=1 on an isolated matching node with the fixture proxy image")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	workerImage := os.Getenv("CODEX_DOCK_KATA_TEST_IMAGE")
	if workerImage == "" {
		workerImage = "codex-dock:latest"
	}
	proxyImage := os.Getenv("CODEX_DOCK_KATA_API_PROXY_IMAGE")
	if proxyImage == "" {
		proxyImage = "codex-dock-proxy:api-fixture"
	}
	engineClient, err := engine.NewClient()
	if err != nil {
		t.Fatal(err)
	}
	defer engineClient.Close()
	if os.Getenv("CODEX_DOCK_KATA_API_OWN_IMAGE") == "1" {
		defer func() {
			cleanup, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			if _, err := engineClient.ImageRemove(cleanup, proxyImage, image.RemoveOptions{}); err != nil {
				t.Error("fixture image cleanup:", err)
			} else if _, err := engineClient.ImageInspect(cleanup, proxyImage); !errdefs.IsNotFound(err) {
				t.Errorf("fixture image reference survived cleanup: %v", err)
			}
		}()
	}
	for _, mode := range []string{"compact", "lifecycle", "save-failure", "conflict", "restart"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			if err := os.Chmod(root, 0700); err != nil {
				t.Fatal(err)
			}
			project := filepath.Join(root, "project")
			if err := os.Mkdir(project, 0755); err != nil {
				t.Fatal(err)
			}
			store := &apiFixtureStore{mode: mode, creds: credentialstore.Credentials{Version: "version-1", ConnectionVersion: "connection-1", OpenAIOAuth: &credentialstore.OpenAIOAuth{AccessToken: "fixture-access-initial", RefreshToken: "fixture-refresh-initial"}}}
			storeCtx, stopStore := context.WithCancel(ctx)
			done := make(chan error, 1)
			socket := filepath.Join(root, "c.sock")
			go func() { done <- credentialstore.Serve(storeCtx, socket, "fixture-case", store) }()
			defer func() {
				stopStore()
				if err := <-done; err != nil {
					t.Error(err)
				}
			}()
			awaitAPI(t, ctx, func() bool { _, err := os.Stat(socket); return err == nil })
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			port := ln.Addr().(*net.TCPAddr).Port
			_ = ln.Close()
			proxyOpts := CaseProxyOptions{NodeSocket: engine.Socket(), Name: fmt.Sprintf("cxd-api-%x", time.Now().UnixNano()), CaseID: "fixture-case", StoreSocket: socket, AdminPort: port, AdminSecret: strings.Repeat("fixture-", 5), Image: proxyImage, AllowedModels: []string{"allowed"}}
			proxy, err := EnsureCaseProxy(ctx, proxyOpts)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				cleanup, cancel := context.WithTimeout(context.Background(), time.Minute)
				defer cancel()
				if err := RemoveCaseProxy(cleanup, engine.Socket(), proxy.Name, "fixture-case"); err != nil {
					t.Error(err)
				}
			}()
			c, err := Open(ctx, Options{NodeSocket: engine.Socket(), ProxyAdminURL: proxy.AdminURL, ProxyContainerURL: proxy.ContainerURL, ProxyAdminSecret: proxyOpts.AdminSecret, StateDir: filepath.Join(root, "state")})
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			if _, err := engineClient.ImageRemove(ctx, proxyImage, image.RemoveOptions{}); !errdefs.IsConflict(err) {
				t.Fatalf("referenced proxy image removal was not rejected: %v", err)
			}
			caps, err := c.ProxyCapabilities(ctx)
			if err != nil || !caps.ModelAuthorization || !caps.ModelPolicyActive {
				t.Fatalf("model capabilities: %+v %v", caps, err)
			}
			identity := identity.Identity{CaseID: "fixture-case", RunID: "run-" + mode, ConnectionVersion: "connection-1", Provider: "openai"}
			req := LaunchRequest{RequestID: proxy.Name + "/worker", ProjectDir: project, Image: workerImage, Profile: "small", NoInternet: true, User: strconv.Itoa(os.Getuid()) + ":" + strconv.Itoa(os.Getgid()), Identity: identity}
			if mode == "compact" {
				req.Profile = "compact"
			}
			if mode == "lifecycle" {
				req.Lifecycle.IdleTimeout = 500 * time.Millisecond
				req.Environment = EnvironmentPolicy{StartHooks: [][]string{{"sh", "-c", "printf ready > /workspace/hook"}}, Readiness: []string{"test", "-f", "/workspace/hook"}}
			}
			w, err := c.Create(ctx, req)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				cleanup, cancel := context.WithTimeout(context.Background(), time.Minute)
				defer cancel()
				if err := c.Remove(cleanup, w.ID, true); err != nil {
					t.Error(err)
				}
			}()
			if w.Status != "created" {
				t.Fatalf("create started worker: %s", w.Status)
			}
			if _, err := os.Stat(filepath.Join(project, "hook")); !os.IsNotExist(err) {
				t.Fatal("hook ran before Start")
			}
			again, err := c.Create(ctx, req)
			if err != nil || again.ID != w.ID {
				t.Fatalf("create replay: %+v %v", again, err)
			}
			if _, err := c.Launch(ctx, req); err == nil {
				t.Fatal("Create and Launch shared a request identity")
			}
			if err := c.Start(ctx, w.ID); err != nil {
				t.Fatal(err)
			}
			usage, err := c.ResourceUsage(ctx, w.ID)
			if err != nil || usage.OOMKills != 0 {
				t.Fatalf("resource usage: %+v %v", usage, err)
			}
			t.Logf("%s idle: host current=%d peak=%d guest budget=%d host ceiling=%d", req.Profile, usage.HostMemoryCurrentBytes, usage.HostMemoryPeakBytes, usage.Profile.GuestMemoryBytes, usage.Profile.HostMemoryBytes)
			request := func(path, body string) string {
				t.Helper()
				var out, stderr bytes.Buffer
				code, err := c.Exec(ctx, w.ID, ExecRequest{Argv: []string{"python3", "-c", apiHTTPRequest, path, body}, Stdout: &out, Stderr: &stderr})
				if err != nil || code != 0 {
					t.Fatalf("guest HTTP: code=%d err=%v stderr=%s", code, err, stderr.String())
				}
				return strings.TrimSpace(out.String())
			}
			if got := request("/v1/responses", `{"model":"other"}`); got != "403" {
				t.Fatalf("model bypass: %s", got)
			}
			if got := request("/v1/responses", `{"model":"allowed"}`); got != "204" {
				t.Fatalf("allowed model: %s", got)
			}
			if mode == "compact" {
				job, err := c.StartJob(ctx, w.ID, ExecRequest{Argv: []string{"python3", "-c", "import time; x=bytearray(256*1024*1024); print('allocated',flush=True); time.sleep(60)"}})
				if err != nil {
					t.Fatal(err)
				}
				awaitAPI(t, ctx, func() bool {
					logs, err := c.JobLogs(ctx, w.ID, job.ID, 0)
					return err == nil && strings.Contains(logs.Output, "allocated")
				})
				otherReq := req
				otherReq.RequestID += "/concurrent"
				otherReq.Identity.RunID = "compact-concurrent"
				other, err := c.Launch(ctx, otherReq)
				if err != nil {
					t.Fatal(err)
				}
				defer func() {
					cleanup, cancel := context.WithTimeout(context.Background(), time.Minute)
					defer cancel()
					if err := c.Remove(cleanup, other.ID, true); err != nil {
						t.Error(err)
					}
				}()
				for _, id := range []string{w.ID, other.ID} {
					u, err := c.ResourceUsage(ctx, id)
					if err != nil || u.OOMKills != 0 || u.HostMemoryCurrentBytes >= u.Profile.HostMemoryBytes {
						t.Fatalf("compact envelope: %+v %v", u, err)
					}
					var out bytes.Buffer
					code, err := c.Exec(ctx, id, ExecRequest{Argv: []string{"sh", "-c", "cat /sys/fs/cgroup/memory.max; cat /sys/fs/cgroup/memory.events"}, Stdout: &out})
					lines := strings.Split(strings.TrimSpace(out.String()), "\n")
					if err != nil || code != 0 || lines[0] != strconv.FormatInt(512<<20, 10) {
						t.Fatalf("compact guest bound: %q %v", out.String(), err)
					}
					guestOOMFound := false
					for _, line := range lines[1:] {
						fields := strings.Fields(line)
						if len(fields) == 2 && (fields[0] == "oom" || fields[0] == "oom_kill") {
							if fields[1] != "0" {
								t.Fatalf("compact guest OOM: %s", line)
							}
							if fields[0] == "oom_kill" {
								guestOOMFound = true
							}
						}
					}
					if !guestOOMFound {
						t.Fatal("guest OOM observation missing")
					}
					t.Logf("compact concurrent: current=%d peak=%d OOM=%d", u.HostMemoryCurrentBytes, u.HostMemoryPeakBytes, u.OOMKills)
				}
				if current, err := c.Job(ctx, w.ID, job.ID); err != nil || !current.Running {
					t.Fatalf("compact load job exited unexpectedly: %+v %v", current, err)
				}
				if err := c.KillJob(ctx, w.ID, job.ID); err != nil {
					t.Fatal(err)
				}
				return
			}
			if mode == "lifecycle" {
				job, err := c.StartJob(ctx, w.ID, ExecRequest{Argv: []string{"python3", "-c", "import time; x=bytearray(256*1024*1024); print('allocated',flush=True); time.sleep(60)"}})
				if err != nil {
					t.Fatal(err)
				}
				awaitAPI(t, ctx, func() bool {
					logs, err := c.JobLogs(ctx, w.ID, job.ID, 0)
					return err == nil && strings.Contains(logs.Output, "allocated")
				})
				time.Sleep(750 * time.Millisecond)
				actions, err := c.Reap(ctx, false)
				if err != nil || len(actions) != 0 {
					t.Fatalf("running job reaped: %+v %v", actions, err)
				}
				usage, err = c.ResourceUsage(ctx, w.ID)
				if err != nil || usage.OOMKills != 0 {
					t.Fatalf("loaded usage: %+v %v", usage, err)
				}
				t.Logf("small +256MiB job: host current=%d peak=%d OOM=%d", usage.HostMemoryCurrentBytes, usage.HostMemoryPeakBytes, usage.OOMKills)
				if err := c.KillJob(ctx, w.ID, job.ID); err != nil {
					t.Fatal(err)
				}
				awaitAPI(t, ctx, func() bool { j, err := c.Job(ctx, w.ID, job.ID); return err == nil && !j.Running })
				streamDone := make(chan error, 1)
				go func() {
					code, err := c.Exec(ctx, w.ID, ExecRequest{Argv: []string{"python3", "-c", apiHTTPStream}, Stdout: io.Discard, Stderr: io.Discard})
					if err == nil && code != 0 {
						err = fmt.Errorf("stream exit %d", code)
					}
					streamDone <- err
				}()
				awaitAPI(t, ctx, func() bool { _, err := os.Stat(filepath.Join(project, "stream-started")); return err == nil })
				time.Sleep(750 * time.Millisecond)
				if actions, err := c.Reap(ctx, false); err != nil || len(actions) != 0 {
					t.Fatalf("streaming exec reaped: %+v %v", actions, err)
				}
				if err := c.RevokeRun(ctx, identity); err != nil {
					t.Fatal(err)
				}
				select {
				case err := <-streamDone:
					if err != nil {
						t.Fatal(err)
					}
				case <-time.After(15 * time.Second):
					t.Fatal("revocation did not close stream")
				}
				if got := request("/v1/responses", `{"model":"allowed"}`); got != "401" {
					t.Fatalf("revoked token reused: %s", got)
				}
				time.Sleep(750 * time.Millisecond)
				if actions, err := c.Reap(ctx, false); err != nil || len(actions) != 1 || actions[0].Reason != "idle_timeout" {
					t.Fatalf("idle worker not collected: %+v %v", actions, err)
				}
				// Launch cancellation after a hook began must stop the VM, revoke
				// credentials and leave its durable request reconcilable.
				cancelReq := req
				cancelReq.RequestID = proxy.Name + "/cancel"
				cancelReq.Identity.RunID = "cancel-run"
				cancelReq.Lifecycle = LifecyclePolicy{}
				cancelReq.Environment = EnvironmentPolicy{StartHooks: [][]string{{"sh", "-c", "touch /workspace/launch-started; sleep 120"}}, Readiness: []string{"false"}, ReadinessTimeout: 150 * time.Second}
				launchCtx, stopLaunch := context.WithCancel(ctx)
				launched := make(chan error, 1)
				go func() { _, err := c.Launch(launchCtx, cancelReq); launched <- err }()
				awaitAPI(t, ctx, func() bool { _, err := os.Stat(filepath.Join(project, "launch-started")); return err == nil })
				stopLaunch()
				select {
				case err := <-launched:
					if !errors.Is(err, context.Canceled) {
						t.Fatalf("cancel: %v", err)
					}
				case <-time.After(60 * time.Second):
					t.Fatal("launch cleanup timed out")
				}
				status, err := c.Request(ctx, cancelReq.RequestID)
				if err != nil || status.State == "pending" || status.State == "cleanup-pending" {
					t.Fatalf("unreconciled cancellation: %+v %v", status, err)
				}
				workers, err := c.List(ctx)
				if err != nil {
					t.Fatal(err)
				}
				for _, other := range workers {
					if other.ID != w.ID {
						t.Fatalf("canceled VM retained: %+v", other)
					}
				}
				inventory, err := c.node.Inventory(ctx)
				if err != nil || !inventory.EngineAvailable {
					t.Fatalf("post-cancel inventory: %v", err)
				}
				for _, vm := range inventory.VMs {
					if vm.Allocated || vm.Populated || vm.ContainerID != w.ID && vm.ContainerPresent {
						t.Fatalf("canceled host resources survived: %+v", vm)
					}
				}
				return
			}
			refreshPath := "/oauth/token?cdx="
			if got := request(refreshPath, `{"grant_type":"refresh_token"}`); mode == "restart" && got != "200" || mode != "restart" && got != "502" {
				t.Fatalf("refresh %s: %s", mode, got)
			}
			store.mu.Lock()
			saves := store.saves
			creds := store.creds
			loads := store.loads
			store.mu.Unlock()
			if saves != 1 {
				t.Fatalf("refresh save count %d", saves)
			}
			if mode != "restart" {
				if creds.OpenAIOAuth.AccessToken != "fixture-access-initial" {
					t.Fatal("failed CAS/save published credentials")
				}
				if got := request("/v1/responses", `{"model":"allowed"}`); got != "401" {
					t.Fatalf("failed refresh retained token: %s", got)
				}
				return
			}
			if creds.OpenAIOAuth.AccessToken != "fixture-access-rotated" {
				t.Fatal("refresh not persisted")
			}
			seconds := 5
			if err := engineClient.ContainerStop(ctx, proxy.Name, container.StopOptions{Timeout: &seconds}); err != nil {
				t.Fatal(err)
			}
			if _, err := EnsureCaseProxy(ctx, proxyOpts); err != nil {
				t.Fatal(err)
			}
			store.mu.Lock()
			reloaded := store.loads > loads
			store.mu.Unlock()
			if !reloaded {
				t.Fatal("proxy did not reload durable credentials")
			}
			if got := request("/v1/responses", `{"model":"allowed"}`); got != "401" {
				t.Fatalf("proxy restart retained old token: %s", got)
			}
			if err := c.Stop(ctx, w.ID, 5*time.Second); err != nil {
				t.Fatal(err)
			}
			if err := c.Start(ctx, w.ID); err != nil {
				t.Fatal(err)
			}
			if got := request("/v1/responses", `{"model":"allowed"}`); got != "204" {
				t.Fatalf("restored worker: %s", got)
			}
		})
	}
}

func awaitAPI(t *testing.T, ctx context.Context, ready func() bool) {
	t.Helper()
	timer := time.NewTimer(45 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if ready() {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-timer.C:
			t.Fatal("fixture readiness timeout")
		case <-ticker.C:
		}
	}
}

const apiHTTPRequest = `import os,sys,urllib.request,urllib.error
path=sys.argv[1]
if path.endswith('?cdx='): path+=os.environ['CODEX_TOKEN']
req=urllib.request.Request(os.environ['CODEX_AUTH_PROXY_URL']+path,data=sys.argv[2].encode(),headers={'Authorization':'Bearer '+os.environ['CODEX_TOKEN'],'Content-Type':'application/json'})
try:
 with urllib.request.urlopen(req,timeout=15) as resp: print(resp.status)
except urllib.error.HTTPError as e: print(e.code)
`

const apiHTTPStream = `import os,urllib.request
req=urllib.request.Request(os.environ['CODEX_AUTH_PROXY_URL']+'/v1/responses',data=b'{"model":"allowed","stream":true}',headers={'Authorization':'Bearer '+os.environ['CODEX_TOKEN'],'Content-Type':'application/json'})
with urllib.request.urlopen(req,timeout=20) as resp:
 assert resp.readline().startswith(b'data:')
 open('/workspace/stream-started','w').close()
 try:
  while resp.read(1): pass
 except (OSError,ValueError): pass
`
