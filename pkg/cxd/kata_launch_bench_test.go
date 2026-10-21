package cxd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/containerd/errdefs"
	dn "github.com/docker/docker/api/types/network"
	"github.com/pacificbelt30/codex-dock/internal/engine"
	"github.com/pacificbelt30/codex-dock/internal/network"
	"github.com/pacificbelt30/codex-dock/pkg/cxd/credentialstore"
	"github.com/pacificbelt30/codex-dock/pkg/cxd/identity"
)

// BenchmarkKataLaunch owns synthetic credentials, a case proxy and fresh workers.
// Run only on an isolated prepared node, with no concurrent launches. Images and
// proxy must already be available; no real account or upstream AI is used.
func BenchmarkKataLaunch(b *testing.B) {
	if os.Getenv("CODEX_DOCK_BENCH_LAUNCH") != "1" {
		b.Skip("set CODEX_DOCK_BENCH_LAUNCH=1 on an isolated prepared Kata node")
	}
	b.StopTimer()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	node, err := engine.NewClient()
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = node.Close() })
	if _, err := node.NetworkInspect(ctx, network.EgressNetworkName, dn.InspectOptions{}); errdefs.IsNotFound(err) {
		b.Cleanup(func() {
			clean, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			if err := node.NetworkRemove(clean, network.EgressNetworkName); err != nil && !errdefs.IsNotFound(err) {
				b.Error("removing benchmark egress network:", err)
			}
		})
	} else if err != nil {
		b.Fatal(err)
	}
	root := b.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		b.Fatal(err)
	}
	caseID := fmt.Sprintf("launch-bench-%x", time.Now().UnixNano())
	storeCtx, stopStore := context.WithCancel(ctx)
	done := make(chan error, 1)
	socket := filepath.Join(root, "store.sock")
	store := &apiFixtureStore{creds: credentialstore.Credentials{Version: "1", ConnectionVersion: "1", OpenAIAPIKey: "sk-launch-benchmark-fixture-never-valid"}}
	go func() { done <- credentialstore.Serve(storeCtx, socket, caseID, store) }()
	b.Cleanup(func() {
		stopStore()
		if err := <-done; err != nil {
			b.Error(err)
		}
	})
	for attempt := 0; ; attempt++ {
		if info, err := os.Stat(socket); err == nil && info.Mode().Perm() == 0600 {
			break
		}
		if attempt == 200 {
			b.Fatal("benchmark credential store did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	proxyImage := os.Getenv("CODEX_DOCK_KATA_PROXY_IMAGE")
	if proxyImage == "" {
		proxyImage = "codex-dock-proxy:latest"
	}
	secret := strings.Repeat("launch-benchmark-fixture-", 2)
	b.Cleanup(func() {
		clean, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if err := RemoveCaseProxy(clean, engine.Socket(), caseID, caseID); err != nil {
			b.Error("removing benchmark proxy:", err)
		}
	})
	proxy, err := EnsureCaseProxy(ctx, CaseProxyOptions{NodeSocket: engine.Socket(), Name: caseID, CaseID: caseID, StoreSocket: socket, AdminPort: port, AdminSecret: secret, Image: proxyImage})
	if err != nil {
		b.Fatal(err)
	}
	c, err := Open(ctx, Options{NodeSocket: engine.Socket(), ProxyAdminURL: proxy.AdminURL, ProxyContainerURL: proxy.ContainerURL, ProxyAdminSecret: secret, StateDir: filepath.Join(root, "state")})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = c.Close() })
	image := os.Getenv("CODEX_DOCK_KATA_TEST_IMAGE")
	if image == "" {
		image = "codex-dock:latest"
	}
	profile := os.Getenv("CODEX_DOCK_BENCH_PROFILE")
	if profile == "" {
		profile = "compact"
	}
	var total, firstExec time.Duration
	phases := make(map[string]time.Duration)
	for i := 0; i < b.N; i++ {
		req := LaunchRequest{RequestID: fmt.Sprintf("%s/%d", caseID, i), ProjectDir: b.TempDir(), Image: image, Profile: profile, NoInternet: true, NoGitMetadata: true, User: fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()), Identity: identity.Identity{CaseID: caseID, RunID: fmt.Sprintf("run-%d", i), ConnectionVersion: "1", Provider: "openai"}}
		b.StartTimer()
		worker, timing, launchErr := c.LaunchWithTimings(ctx, req)
		b.StopTimer()
		// Retain failed-launch IDs too; a failed cleanup still has a durable owner.
		if worker.ID != "" {
			b.Cleanup(func() {
				clean, cancel := context.WithTimeout(context.Background(), time.Minute)
				defer cancel()
				if err := c.Remove(clean, worker.ID, true); err != nil && !errors.Is(err, ErrNotFound) {
					b.Error("removing benchmark worker:", err)
				}
			})
		}
		data, err := json.Marshal(timing)
		if err != nil {
			b.Fatal(err)
		}
		b.Logf("launch_timing %s", data)
		if launchErr != nil {
			b.Fatal("measured launch:", launchErr)
		}
		total += timing.Duration
		for _, phase := range timing.Phases {
			phases[phase.Name] += phase.Duration
		}
		start := time.Now()
		code, execErr := c.Exec(ctx, worker.ID, ExecRequest{Argv: []string{"/bin/true"}})
		firstExec += time.Since(start)
		if execErr != nil || code != 0 {
			b.Fatalf("first exec: exit=%d error=%v", code, execErr)
		}
		if err := c.Remove(ctx, worker.ID, true); err != nil {
			b.Fatal("removing measured worker:", err)
		}
	}
	ms := float64(time.Millisecond) * float64(b.N)
	b.ReportMetric(float64(total)/ms, "launch_ms/op")
	b.ReportMetric(float64(firstExec)/ms, "first_exec_ms/op")
	for name, elapsed := range phases {
		b.ReportMetric(float64(elapsed)/ms, name+"_ms/op")
	}
}
