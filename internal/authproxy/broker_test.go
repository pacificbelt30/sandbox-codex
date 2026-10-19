package authproxy

import (
	"context"
	"github.com/pacificbelt30/codex-dock/pkg/cxd/identity"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestBrokerScopesLiveTokensAndDoesNotFollowRedirects(t *testing.T) {
	var hits, secretCalls atomic.Int32
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Header.Get("X-Service-Key") != "Bearer host-secret" {
			t.Error("missing injected credential")
		}
		if r.Header.Get("Authorization") != "" {
			t.Error("placeholder leaked")
		}
		if r.URL.Path == "/api/redirect" {
			http.Redirect(w, r, "https://other.example/leak", http.StatusFound)
			return
		}
		_, _ = io.WriteString(w, "ok")
	}))
	defer upstream.Close()
	route := CredentialRoute{Name: "service", Upstream: upstream.URL, Paths: []string{"/api"}, Methods: []string{"GET"}, Header: "X-Service-Key", Prefix: "Bearer ", Provider: "openai", Secret: func(context.Context) (string, error) { secretCalls.Add(1); return "host-secret", nil }}
	p, err := NewProxy(Config{BrokerOnly: true, CredentialRoutes: []CredentialRoute{route}})
	if err != nil {
		t.Fatal(err)
	}
	p.httpClient = upstream.Client()
	id := identity.Identity{CaseID: "case", RunID: "run", ConnectionVersion: "1", Provider: "openai"}
	token, err := p.IssueTokenWithIdentity("worker", 60, id)
	if err != nil {
		t.Fatal(err)
	}
	// Caller mutation must not widen the fixed route.
	route.Paths[0] = "/"
	for _, tt := range []struct {
		path, token, method string
		status              int
	}{{"/broker/service/api/item", token, "GET", 200}, {"/broker/service/private", token, "GET", 403}, {"/broker/service/api/item", token, "POST", 403}, {"/broker/service/api/item", "forged", "GET", 401}, {"/broker/service/api/%2e%2e/private", token, "GET", 403}, {"/broker/service/api/redirect", token, "GET", 302}} {
		r := httptest.NewRequest(tt.method, tt.path, nil)
		r.Header.Set("Authorization", "Bearer "+tt.token)
		r.Header.Set("X-Service-Key", "attacker")
		w := httptest.NewRecorder()
		p.handleBroker(w, r)
		if w.Code != tt.status {
			t.Errorf("%s: got %d body %s", tt.path, w.Code, w.Body.String())
		}
	}
	if hits.Load() != 2 || secretCalls.Load() != 2 {
		t.Fatalf("hits=%d secrets=%d", hits.Load(), secretCalls.Load())
	}
	if err = p.RevokeRun(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/broker/service/api/item", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	p.handleBroker(w, r)
	if w.Code != 401 || hits.Load() != 2 {
		t.Fatal("revoked token reached upstream")
	}
}
func TestBrokerRejectsUnsafeConfiguration(t *testing.T) {
	for _, r := range []CredentialRoute{{Name: "x", Upstream: "http://example.com"}, {Name: "x", Upstream: "https://u:p@example.com"}} {
		if err := validateRoutes([]CredentialRoute{r}); err == nil {
			t.Fatal("unsafe route accepted")
		}
	}
	if _, err := NewProxy(Config{Role: RoleEgress, CredentialRoutes: []CredentialRoute{{}}}); err == nil {
		t.Fatal("egress accepted broker")
	}
}
