package sandbox

import (
	"context"
	"encoding/json"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/pacificbelt30/codex-dock/internal/workerpolicy"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestReapTouchesOnlyExplicitExpiredPolicies(t *testing.T) {
	now := time.Now().UTC()
	encoded, _ := json.Marshal(workerpolicy.Lifecycle{MaxLifetime: time.Hour})
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/v1.51")
		switch path {
		case "/containers/json":
			_ = json.NewEncoder(w).Encode([]container.Summary{{ID: "expired", Names: []string{"/expired"}}, {ID: "legacy", Names: []string{"/legacy"}}})
		case "/containers/expired/json", "/containers/legacy/json":
			labels := map[string]string{labelManaged: "true"}
			if strings.Contains(path, "expired") {
				labels[lifecycleLabel] = string(encoded)
			}
			_ = json.NewEncoder(w).Encode(container.InspectResponse{ContainerJSONBase: &container.ContainerJSONBase{ID: strings.Split(path, "/")[2], Created: now.Add(-2 * time.Hour).Format(time.RFC3339Nano), State: &container.State{Running: true, Status: "running"}}, Config: &container.Config{Labels: labels}})
		default:
			calls++
			t.Errorf("unexpected mutation %s", path)
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	cli, err := client.NewClientWithOpts(client.WithHost(server.URL), client.WithVersion("1.51"), client.WithHTTPClient(server.Client()))
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()
	m := &Manager{cli: cli}
	actions, err := m.Reap(context.Background(), now, true)
	if err != nil || len(actions) != 1 || actions[0].Worker != "expired" || actions[0].Reason != "max_lifetime" || calls != 0 {
		t.Fatalf("%+v %v calls=%d", actions, err, calls)
	}
}
