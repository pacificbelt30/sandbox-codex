package cxd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

func verifyQuarantineAPI(t *testing.T, ctx context.Context, c *Client, node *client.Client, opts Options, req LaunchRequest, w Worker, project string) {
	t.Helper()
	siblingReq := req
	siblingReq.RequestID += "/sibling"
	siblingReq.Identity.RunID += "-sibling"
	siblingReq.ProjectDir = filepath.Join(project, "sibling")
	if err := os.Mkdir(siblingReq.ProjectDir, 0755); err != nil {
		t.Fatal(err)
	}
	sibling, err := c.Launch(ctx, siblingReq)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		clean, stop := context.WithTimeout(context.Background(), time.Minute)
		defer stop()
		if err := c.Remove(clean, sibling.ID, true); err != nil {
			t.Error(err)
		}
	}()
	selected, err := c.Policy(ctx, w.ID)
	if err != nil {
		t.Fatal(err)
	}
	otherPolicy, err := c.Policy(ctx, sibling.ID)
	if err != nil || !selected.GeneralEgress || selected.Proxy != otherPolicy.Proxy {
		t.Fatalf("workers did not share egress fixture: %+v %+v %v", selected, otherPolicy, err)
	}
	probeSibling := func() {
		t.Helper()
		var out bytes.Buffer
		code, err := c.Exec(ctx, sibling.ID, ExecRequest{Argv: []string{"python3", "-c", apiQuarantineEgress, "probe"}, Stdout: &out, Stderr: &out})
		if err != nil || code != 0 {
			t.Fatalf("sibling general HTTP/CONNECT interrupted: %d %v %s", code, err, &out)
		}
		out.Reset()
		code, err = c.Exec(ctx, sibling.ID, ExecRequest{Argv: []string{"python3", "-c", apiHTTPRequest, "/v1/responses", `{"model":"allowed"}`}, Stdout: &out, Stderr: &out})
		if err != nil || code != 0 || strings.TrimSpace(out.String()) != "204" {
			t.Fatalf("sibling AI interrupted: %d %v %s", code, err, &out)
		}
	}
	probeSibling()
	general, err := c.StartJob(ctx, w.ID, ExecRequest{Argv: []string{"python3", "-c", apiQuarantineEgress, "hold"}})
	if err != nil {
		t.Fatal(err)
	}
	awaitAPI(t, ctx, func() bool { _, err := os.Stat(filepath.Join(project, "egress-started")); return err == nil })
	stream, err := c.StartJob(ctx, w.ID, ExecRequest{Argv: []string{"python3", "-c", apiHTTPStream}})
	if err != nil {
		t.Fatal(err)
	}
	awaitAPI(t, ctx, func() bool { _, err := os.Stat(filepath.Join(project, "stream-started")); return err == nil })
	if err := c.Quarantine(ctx, w.ID); err != nil {
		t.Fatal(err)
	}
	for _, job := range []Job{general, stream} {
		awaitAPI(t, ctx, func() bool { v, err := c.Job(ctx, w.ID, job.ID); return err == nil && !v.Running })
	}
	inspect, err := node.ContainerInspect(ctx, w.ID)
	if err != nil || inspect.State.Running {
		t.Fatalf("quarantine left VM running: %v", err)
	}
	policy, err := c.Policy(ctx, w.ID)
	if err != nil || !policy.Quarantined || policy.GeneralEgress {
		t.Fatalf("quarantine status: %+v %v", policy, err)
	}
	decision, err := c.ExplainPolicy(ctx, w.ID, "203.0.113.1:443", false)
	if err != nil || decision.Allowed || decision.Code != "worker_quarantined" {
		t.Fatalf("quarantine explanation: %+v %v", decision, err)
	}
	if err := c.Start(ctx, w.ID); err == nil {
		t.Fatal("Manager restarted quarantined worker")
	}
	if err := node.ContainerStart(ctx, w.ID, container.StartOptions{}); !errdefs.IsConflict(err) {
		t.Fatalf("direct node start bypassed quarantine: %v", err)
	}
	// A request replay retains this worker; it cannot silently clear quarantine.
	replay, err := c.Create(ctx, req)
	if err != nil || replay.ID != w.ID {
		t.Fatalf("request replay changed quarantined identity: %+v %v", replay, err)
	}
	if requestDir := os.Getenv("CODEX_DOCK_NODE_RESTART_REQUEST_DIR"); requestDir != "" {
		if err := os.Remove(filepath.Join(requestDir, "complete")); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(requestDir, "request"), []byte("restart\n"), 0600); err != nil {
			t.Fatal(err)
		}
		awaitAPI(t, ctx, func() bool { _, err := os.Stat(filepath.Join(requestDir, "complete")); return err == nil })
		if err := node.ContainerStart(ctx, w.ID, container.StartOptions{}); !errdefs.IsConflict(err) {
			t.Fatalf("node restart cleared quarantine: %v", err)
		}
	}
	if binary := os.Getenv("CODEX_DOCK_KATA_TEST_BINARY"); binary != "" {
		config := filepath.Join(filepath.Dir(project), "cli.toml")
		if err := os.WriteFile(config, []byte("[kata]\nnode_socket = "+strconv.Quote(opts.NodeSocket)+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		command := exec.CommandContext(ctx, binary, "--config", config, "policy", "quarantine", "--worker", w.ID, "--json")
		command.Env = append(os.Environ(), "CODEX_DOCK_PROXY_ADMIN_SECRET="+opts.ProxyAdminSecret, "CODEX_DOCK_STATE_DIR="+filepath.Join(filepath.Dir(project), "cli-state"))
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("quarantine CLI: %v %s", err, output)
		}
		var ack struct {
			Quarantined    bool `json:"quarantined"`
			RestartAllowed bool `json:"restart_allowed"`
		}
		if err = json.Unmarshal(output, &ack); err != nil || !ack.Quarantined || ack.RestartAllowed {
			t.Fatalf("CLI acknowledgment: %s %v", output, err)
		}
	}
	if err := c.Quarantine(ctx, w.ID); err != nil {
		t.Fatalf("quarantine retry: %v", err)
	}
	probeSibling()
	// A different client/state directory sees the same node-owned prohibition.
	opts.StateDir = filepath.Join(filepath.Dir(project), "new-state")
	opts.EgressPolicy = EgressPolicy{DefaultDeny: true}
	fresh, err := Open(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	if err := fresh.Start(ctx, w.ID); err == nil {
		t.Fatal("new client cleared quarantine")
	}
	replacementReq := req
	replacementReq.RequestID += "/replacement"
	replacementReq.Identity.RunID += "-replacement"
	replacement, err := fresh.Launch(ctx, replacementReq)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		clean, stop := context.WithTimeout(context.Background(), time.Minute)
		defer stop()
		if err := fresh.Remove(clean, replacement.ID, true); err != nil {
			t.Error(err)
		}
	}()
	if replacement.ID == w.ID {
		t.Fatal("replacement reused quarantined identity")
	}
	replacementPolicy, err := fresh.Policy(ctx, replacement.ID)
	if err != nil || replacementPolicy.GeneralEgress || replacementPolicy.Quarantined {
		t.Fatalf("new strict policy not applied: %+v %v", replacementPolicy, err)
	}
	var out bytes.Buffer
	code, err := fresh.Exec(ctx, replacement.ID, ExecRequest{Argv: []string{"python3", "-c", "import os; assert not os.environ.get('HTTP_PROXY'); assert os.path.exists('/workspace/egress-started')"}, Stdout: &out, Stderr: &out})
	if err != nil || code != 0 {
		t.Fatalf("workspace/new policy: %d %v %s", code, err, &out)
	}
	if _, err := fresh.Get(ctx, w.ID); err != nil {
		t.Fatalf("quarantine did not retain an inspectable worker: %v", err)
	}
	t.Log("verified active general HTTP/CONNECT and AI jobs terminated with target VM; shared-proxy sibling unaffected; durable restart denial, node restart, retry and replacement under a new policy with retained workspace")
}

const apiQuarantineEgress = `import os,socket,urllib.parse,http.client,sys,time
u=urllib.parse.urlsplit(os.environ['HTTP_PROXY'])
s=socket.create_connection((u.hostname,u.port),timeout=15)
s.sendall(b'CONNECT 203.0.113.1:443 HTTP/1.1\r\nHost: 203.0.113.1:443\r\n\r\n')
f=s.makefile('rb')
assert f.readline().split()[1] == b'200'
while f.readline() != b'\r\n': pass
s.sendall(b'echo\n')
assert f.read(5) == b'echo\n'
h=http.client.HTTPConnection(u.hostname,u.port,timeout=15)
h.request('GET','http://203.0.113.1:80/stream')
r=h.getresponse()
assert r.status == 200 and r.readline() == b'ready\n'
if sys.argv[1] == 'hold':
 open('/workspace/egress-started','w').close()
 time.sleep(120)
r.close(); h.close(); f.close(); s.close()
`
