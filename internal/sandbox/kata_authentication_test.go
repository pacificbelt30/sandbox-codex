package sandbox

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	dockernetwork "github.com/docker/docker/api/types/network"
	"github.com/pacificbelt30/codex-dock/internal/engine"
)

// This test uses fake credential bytes and the real restricted Kata execution
// path. It never logs in to a provider or sends an inference request.
func TestKataAuthenticationSession(t *testing.T) {
	if os.Getenv("CODEX_DOCK_KATA_E2E") != "1" {
		t.Skip("set CODEX_DOCK_KATA_E2E=1 on a prepared Linux/KVM host")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	c, err := engine.NewClient()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	workerImage, proxyImage := os.Getenv("CODEX_DOCK_KATA_TEST_IMAGE"), os.Getenv("CODEX_DOCK_KATA_PROXY_IMAGE")
	if workerImage == "" || proxyImage == "" {
		t.Fatal("explicit preloaded worker/proxy images are required")
	}
	name := "oaiso-auth-test-" + randomSuffix()
	proxyName := name + "-egress"
	net, err := c.NetworkCreate(ctx, name+"-net", dockernetwork.CreateOptions{Driver: "bridge"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), time.Minute)
		defer stop()
		if err := c.ContainerRemove(cleanup, proxyName, container.RemoveOptions{Force: true}); err != nil && !errdefs.IsNotFound(err) {
			t.Error("login proxy cleanup failed:", err)
		}
		if err := c.NetworkRemove(cleanup, net.ID); err != nil && !errdefs.IsNotFound(err) {
			t.Error("login egress network cleanup failed:", err)
		}
	})
	proxy, err := c.ContainerCreate(ctx, &container.Config{Image: proxyImage, Labels: map[string]string{"io.codex-dock.proxy-layout": "authentication-acceptance"}, Cmd: []string{"proxy", "serve", "--role", "egress", "--listen", "0.0.0.0:18082", "--block-private", "--forward-default-deny", "--forward-allow-domain", "=example.com", "--forward-allow-port", "443"}},
		&container.HostConfig{Runtime: "runc", NetworkMode: container.NetworkMode(net.ID), CapDrop: []string{"ALL"}, SecurityOpt: []string{"no-new-privileges:true"}}, nil, nil, proxyName)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.ContainerStart(ctx, proxy.ID, container.StartOptions{}); err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(ManagerConfig{HTTPProxyURL: "http://" + proxyName + ":18082", StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close() })
	id, err := m.RunAuthenticationContext(ctx, name, workerImage, "1000:1000")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), time.Minute)
		defer stop()
		if err := m.RemoveContext(cleanup, id, true); err != nil {
			t.Error("login VM cleanup failed:", err)
		}
	})
	v, err := c.ContainerInspect(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if v.Config.Labels[labelAuthenticationSession] != "true" || !v.HostConfig.ReadonlyRootfs || len(v.HostConfig.Mounts) != 1 || v.HostConfig.Mounts[0].Target != guestGuard || workerPlaceholder(v) != "" {
		t.Fatal("login VM has unexpected shares, credentials or a writable root")
	}
	for _, entry := range v.Config.Env {
		if strings.HasPrefix(entry, "OPENAI_API_KEY=") || strings.HasPrefix(entry, "CLAUDE_CODE_OAUTH_TOKEN=") || strings.HasPrefix(entry, "DOCK_ENVIRONMENT_JSON=") {
			t.Fatal("login VM inherited authentication or hooks")
		}
	}
	secret := "fixture-login-secret-never-valid"
	script := `import os,pathlib,sys
p=pathlib.Path(os.environ['HOME'])/'.codex'
p.mkdir(mode=0o700)
(p/'auth.json').write_text(sys.stdin.read())
os.chmod(p/'auth.json',0o600)
try:
 pathlib.Path('/workspace/login-sentinel').write_text('unexpected')
 raise AssertionError('writable image')
except OSError:
 pass
print('memory-only login file created')
`
	var result bytes.Buffer
	code, err := m.ExecWithOptions(ctx, id, ExecOptions{Argv: []string{"/usr/bin/python3", "-c", script}, WorkingDir: AuthenticationHome, Stdin: strings.NewReader(secret), Stdout: &result, Stderr: &result})
	if err != nil || code != 0 {
		t.Fatal("login tmpfs write/read-only image check failed:", code, err)
	}
	result.Reset()
	code, err = m.ExecWithOptions(ctx, id, ExecOptions{Argv: []string{"/bin/cat", AuthenticationHome + "/.codex/auth.json"}, WorkingDir: AuthenticationHome, Stdout: &result})
	if err != nil || code != 0 || result.String() != secret {
		t.Fatal("private login result could not be collected")
	}
	logs, err := c.ContainerLogs(ctx, id, container.LogsOptions{ShowStdout: true, ShowStderr: true})
	if err != nil {
		t.Fatal(err)
	}
	console, err := io.ReadAll(io.LimitReader(logs, 65536))
	_ = logs.Close()
	if err != nil || bytes.Contains(console, []byte(secret)) || bytes.Contains(console, []byte("memory-only login")) {
		t.Fatal("streaming login output entered persistent worker logs")
	}
	for _, command := range [][]string{{"/usr/local/bin/codex", "--version"}, {"/usr/local/bin/claude", "--version"}} {
		result.Reset()
		code, err := m.ExecWithOptions(ctx, id, ExecOptions{Argv: command, WorkingDir: AuthenticationHome, Stdout: &result, Stderr: &result})
		if err != nil || code != 0 || result.Len() == 0 {
			t.Fatal("fixed official CLI cannot run in login VM:", command[0], code, err)
		}
		t.Logf("login CLI: %s", strings.TrimSpace(result.String()))
	}
	if err := m.StopContext(ctx, id, 2); err != nil {
		t.Fatal(err)
	}
	if err := m.StartContext(ctx, id); err == nil || !strings.Contains(err.Error(), "cannot be resumed") {
		t.Fatal("stopped authentication VM was resumable")
	}
	t.Logf("verified: attested Kata, memory-only home, no workspace/auth bind, streaming output, non-resumable session (%s)", name)
}
