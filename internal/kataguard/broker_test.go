package kataguard

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientRejectsNodesWithoutCoherentSharedWorkspaces(t *testing.T) {
	for _, cache := range []string{"", "auto", "always", "none"} {
		t.Run("cache-"+cache, func(t *testing.T) {
			const endpoint = "unix:///run/fixture.sock"
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request nodeRequest
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Action != "validate" || request.Endpoint != endpoint {
					t.Errorf("unexpected validation request: %+v %v", request, err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				_ = json.NewEncoder(w).Encode(nodeReply{Info: NodeInfo{Profile: Profile, Endpoint: endpoint, Stamp: strings.Repeat("a", 64), Launcher: "/opt/fixture/guard", Root: "/sys/fs/cgroup/fixture", SharedFSCache: cache}})
			}))
			defer s.Close()
			c := &Client{client: s.Client(), endpoint: endpoint}
			// Use the fixture HTTP transport while preserving the real request.
			c.client.Transport = &fixtureNodeTransport{target: s.URL, next: s.Client().Transport}
			err := c.Validate(context.Background(), "io.containerd.kata.v2")
			if cache == "none" {
				if err != nil || c.Stamp() != strings.Repeat("a", 64) {
					t.Fatalf("current node rejected: %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), "shared workspaces") || c.Stamp() != "" {
				t.Fatalf("old/incoherent node accepted: cache=%q error=%v", cache, err)
			}
		})
	}
}

type fixtureNodeTransport struct {
	target string
	next   http.RoundTripper
}

func (f *fixtureNodeTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	request := r.Clone(r.Context())
	base, err := http.NewRequestWithContext(r.Context(), r.Method, f.target+r.URL.Path, nil)
	if err != nil {
		return nil, err
	}
	request.URL = base.URL
	return f.next.RoundTrip(request)
}
