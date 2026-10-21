package authproxy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/pacificbelt30/codex-dock/pkg/cxd/identity"
)

func TestTokenModelPolicyAcrossAdminRestoreAndRevocation(t *testing.T) {
	p := newTestProxy(t)
	p.cfg.AdminSecret = "fixture-admin"
	p.cfg.AllowedModels = []string{"cheap", "costly"}
	mux := http.NewServeMux()
	mux.HandleFunc("/admin/mode", p.handleAdminMode)
	mux.HandleFunc("/admin/issue", p.handleAdminIssue)
	mux.HandleFunc("/admin/restore", p.handleAdminRestore)
	admin := httptest.NewServer(mux)
	defer admin.Close()
	remote, err := NewRemoteProxy(admin.URL, "http://worker-proxy", p.cfg.AdminSecret)
	if err != nil || !remote.SupportsTokenModelPolicy() {
		t.Fatalf("capability: %v", err)
	}
	id := identity.Identity{CaseID: "case", RunID: "run", ConnectionVersion: "connection", Provider: "openai"}
	policy := &identity.ModelPolicy{AllowedModels: []string{"cheap", "outside-global"}}
	cheap, err := remote.IssueTokenWithPolicy("cheap", 60, id, policy)
	if err != nil {
		t.Fatal(err)
	}
	policy.AllowedModels[0] = "costly"
	costly, err := remote.IssueTokenWithPolicy("costly", 60, id, policy)
	if err != nil {
		t.Fatal(err)
	}
	deny, err := remote.IssueTokenWithPolicy("deny", 60, id, &identity.ModelPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	var hits atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1); w.WriteHeader(204) }))
	defer up.Close()
	check := func(token, model string, want int) {
		t.Helper()
		r := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"model":"`+model+`"}`))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		p.reverseProxy(w, r, "/v1", up.URL, func(http.Header) {})
		if w.Code != want {
			t.Fatalf("model %s: got %d want %d", model, w.Code, want)
		}
	}
	check(cheap, "cheap", 204)
	check(cheap, "costly", 403)
	check(costly, "costly", 204)
	check(costly, "cheap", 403)
	check(costly, "outside-global", 403)
	check(deny, "cheap", 403)
	p.RevokeToken("cheap")
	check(cheap, "cheap", 401)
	policy.AllowedModels = []string{"cheap"}
	if err := remote.RestoreTokenWithPolicy("cheap", cheap, 60, id, policy); err != nil {
		t.Fatal(err)
	}
	policy.AllowedModels[0] = "costly"
	check(cheap, "cheap", 204)
	check(cheap, "costly", 403)
	if err := p.RevokeRun(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if err := remote.RestoreTokenWithPolicy("cheap", cheap, 60, id, policy); err == nil {
		t.Fatal("restored revoked run")
	}
	check(cheap, "cheap", 401)
	if hits.Load() != 3 {
		t.Fatalf("denial reached upstream: %d", hits.Load())
	}
}

func TestTokenPolicyWithoutGlobalLimitAndLegacyProxy(t *testing.T) {
	p := newTestProxy(t)
	policy := &identity.ModelPolicy{AllowedModels: []string{"cheap"}}
	token, err := p.IssueTokenWithPolicy("worker", 60, identity.Identity{}, policy)
	if err != nil {
		t.Fatal(err)
	}
	policy.AllowedModels[0] = "costly"
	for _, body := range []string{`{"model":"costly"}`, `{"model":"cheap","Model":"costly"}`} {
		r := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		p.reverseProxy(w, r, "/v1", "http://must-not-dial.invalid", func(http.Header) { t.Error("denied request injected credentials") })
		if w.Code != 403 && w.Code != 400 {
			t.Fatalf("denial: %d", w.Code)
		}
	}
	old := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/admin/mode" {
			t.Error("unsupported policy reached legacy proxy")
		}
		_, _ = io.WriteString(w, `{"worker_token_auth":true}`)
	}))
	defer old.Close()
	remote, err := NewRemoteProxy(old.URL, "http://proxy", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := remote.IssueTokenWithPolicy("worker", 60, identity.Identity{}, policy); err == nil {
		t.Fatal("silently downgraded issue")
	}
	if err := remote.RestoreTokenWithPolicy("worker", token, 60, identity.Identity{}, policy); err == nil {
		t.Fatal("silently downgraded restore")
	}
}

func TestRestrictedTokenCannotUseArbitraryCredentialRoute(t *testing.T) {
	p := newTestProxy(t)
	p.cfg.CredentialRoutes = []CredentialRoute{{Name: "ai", Provider: "openai", Secret: func(context.Context) (string, error) { t.Fatal("denied route read credential"); return "", nil }}}
	token, err := p.IssueTokenWithPolicy("worker", 60, identity.Identity{}, &identity.ModelPolicy{AllowedModels: []string{"cheap"}})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/broker/ai/v1/responses", strings.NewReader(`{"model":"costly"}`))
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	p.handleBroker(w, r)
	if w.Code != 403 {
		t.Fatalf("arbitrary route status: %d", w.Code)
	}
}
