package sandbox

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/system"
	"github.com/docker/docker/client"
	"github.com/pacificbelt30/codex-dock/internal/workerpolicy"
)

func TestQuarantineRequiresUpdatedNodeAndConfirmedTargetStop(t *testing.T) {
	for _, scenario := range []string{"success", "old-node", "node-error", "false-ack", "running-ack", "unmanaged"} {
		t.Run(scenario, func(t *testing.T) {
			id := strings.Repeat("c", 64)
			quarantined := false
			mutations := 0
			proxy := &runtimeProxy{fakeProxy: fakeProxy{endpoint: "http://auth:18080"}}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				path := strings.TrimPrefix(r.URL.Path, "/v1.51")
				switch path {
				case "/containers/target/json", "/containers/" + id + "/json":
					labels := map[string]string{labelManaged: "true", labelIsolation: RuntimeRunc}
					if scenario == "unmanaged" {
						delete(labels, labelManaged)
					}
					labels[workerpolicy.QuarantinedLabel] = "false"
					if quarantined && scenario != "false-ack" {
						labels[workerpolicy.QuarantinedLabel] = "true"
					}
					running := !quarantined || scenario == "running-ack"
					_ = json.NewEncoder(w).Encode(container.InspectResponse{ContainerJSONBase: &container.ContainerJSONBase{ID: id, Name: "/target", HostConfig: &container.HostConfig{}, State: &container.State{Running: running, Status: map[bool]string{true: "running", false: "exited"}[running]}}, Config: &container.Config{Labels: labels, Env: []string{"CODEX_TOKEN=cdx-fixture"}}})
				case "/info":
					var info system.Info
					if scenario != "old-node" {
						info.DriverStatus = [][2]string{{"cxd.quarantine", "1"}}
					}
					_ = json.NewEncoder(w).Encode(info)
				case "/containers/" + id + "/quarantine":
					if r.Method != http.MethodPost {
						t.Errorf("quarantine method %s", r.Method)
					}
					mutations++
					if scenario == "node-error" {
						w.WriteHeader(500)
						return
					}
					quarantined = true
					w.WriteHeader(204)
				default:
					t.Errorf("touched unrelated resource: %s %s", r.Method, path)
					w.WriteHeader(500)
				}
			}))
			defer server.Close()
			endpoint, err := url.Parse(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			httpClient := server.Client()
			httpClient.Transport = quarantineTransport{base: httpClient.Transport, host: endpoint.Host}
			cli, err := client.NewClientWithOpts(client.WithHost(server.URL), client.WithVersion("1.51"), client.WithHTTPClient(httpClient))
			if err != nil {
				t.Fatal(err)
			}
			defer cli.Close()
			m := &Manager{cli: cli, proxy: proxy, stateDir: t.TempDir()}
			if err := os.Chmod(m.stateDir, 0700); err != nil {
				t.Fatal(err)
			}
			err = m.QuarantineContext(context.Background(), "target")
			if (err == nil) != (scenario == "success") {
				t.Fatalf("unexpected result: %v", err)
			}
			expected := 1
			if scenario == "old-node" || scenario == "unmanaged" {
				expected = 0
			}
			if mutations != expected || len(proxy.revoked) != expected {
				t.Fatalf("mutations=%d revocations=%v", mutations, proxy.revoked)
			}
			if scenario == "success" {
				// Repeating quarantine is idempotent; another Manager/state directory
				// still cannot restart a quarantined worker.
				if err = m.QuarantineContext(context.Background(), id); err != nil {
					t.Fatal(err)
				}
				other := &Manager{cli: cli, stateDir: t.TempDir()}
				if err := os.Chmod(other.stateDir, 0700); err != nil {
					t.Fatal(err)
				}
				if err = other.StartContext(context.Background(), id); err == nil || !strings.Contains(err.Error(), "quarantined") {
					t.Fatalf("restart not rejected: %v", err)
				}
			}
		})
	}
}

type quarantineTransport struct {
	base http.RoundTripper
	host string
}

func (t quarantineTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	clone := r.Clone(r.Context())
	clone.URL.Host = t.host
	clone.URL.Scheme = "http"
	return t.base.RoundTrip(clone)
}

func TestStartRechecksQuarantineAfterLifecycleLock(t *testing.T) {
	id := strings.Repeat("d", 64)
	var quarantined atomic.Bool
	inspected := make(chan struct{})
	var first sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/json") {
			t.Errorf("restart bypassed quarantine: %s", r.URL.Path)
			w.WriteHeader(500)
			return
		}
		blocked := quarantined.Load()
		_ = json.NewEncoder(w).Encode(container.InspectResponse{ContainerJSONBase: &container.ContainerJSONBase{ID: id, Name: "/target", HostConfig: &container.HostConfig{}, State: &container.State{Running: !blocked}}, Config: &container.Config{Labels: map[string]string{labelManaged: "true", labelIsolation: RuntimeRunc, workerpolicy.QuarantinedLabel: strconv.FormatBool(blocked)}, Env: []string{"CODEX_TOKEN=cdx-fixture"}}})
		first.Do(func() { close(inspected) })
	}))
	defer server.Close()
	cli, err := client.NewClientWithOpts(client.WithHost(server.URL), client.WithVersion("1.51"), client.WithHTTPClient(server.Client()))
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()
	m := &Manager{cli: cli, stateDir: t.TempDir()}
	if err := os.Chmod(m.stateDir, 0700); err != nil {
		t.Fatal(err)
	}
	release, err := m.lockCleanup(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	unlock := sync.OnceFunc(release)
	defer unlock()
	result := make(chan error, 1)
	go func() { result <- m.StartContext(context.Background(), id) }()
	select {
	case <-inspected:
	case <-time.After(5 * time.Second):
		t.Fatal("initial inspection did not complete")
	}
	quarantined.Store(true)
	unlock()
	select {
	case err := <-result:
		if err == nil || !strings.Contains(err.Error(), "quarantined") {
			t.Fatalf("stale start accepted: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("start did not finish")
	}
}
