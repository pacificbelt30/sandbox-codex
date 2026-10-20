package authproxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestModelPolicyChecksActualBodyBeforeCredentialInjection(t *testing.T) {
	var hits, injected atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"model":"allowed","input":"unchanged"}` || r.Header.Get("Authorization") != "Bearer host-fixture" {
			t.Error("request body or injected credential changed")
		}
		w.WriteHeader(200)
	}))
	defer up.Close()
	p := newTestProxy(t)
	p.cfg.AllowedModels = []string{"allowed"}
	token := issuedTestToken(t, p)
	for _, tc := range []struct {
		name, path, body string
		code             int
	}{
		{"allowed", "/v1/responses", `{"model":"allowed","input":"unchanged"}`, 200},
		{"denied", "/v1/responses", `{"model":"expensive"}`, 403},
		{"missing", "/v1/responses", `{}`, 403},
		{"duplicate", "/v1/responses", `{"model":"allowed","model":"other"}`, 400},
		{"escaped-duplicate", "/v1/responses", `{"model":"allowed","mo\u0064el":"other"}`, 400},
		{"case-variant", "/v1/responses", `{"model":"allowed","Model":"other"}`, 400},
		{"wrong-type", "/v1/responses", `{"model":["allowed"]}`, 400},
		{"trailing", "/v1/responses", `{"model":"allowed"}{}`, 400},
		{"batch-route", "/v1/batches", `{"model":"allowed"}`, 403},
		{"query", "/v1/responses?model=other", `{"model":"allowed"}`, 403},
		{"encoded-route", "/v1/%72esponses", `{"model":"allowed"}`, 403},
		{"oversize", "/v1/responses", strings.Repeat(" ", modelBodyLimit+1), 413},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", tc.path, strings.NewReader(tc.body))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			p.reverseProxy(w, r, "/v1", up.URL, func(h http.Header) { injected.Add(1); h.Set("Authorization", "Bearer host-fixture") })
			if w.Code != tc.code {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
		})
	}
	if hits.Load() != 1 || injected.Load() != 1 {
		t.Fatalf("denial reached upstream/injector: %d/%d", hits.Load(), injected.Load())
	}
}

func TestModelPolicyRejectsWebSocketAndRedirectBypass(t *testing.T) {
	var hits atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Redirect(w, r, "/unrestricted", http.StatusTemporaryRedirect)
	}))
	defer up.Close()
	p := newTestProxy(t)
	p.cfg.AllowedModels = []string{"allowed"}
	token := issuedTestToken(t, p)
	for _, ws := range []bool{false, true} {
		r := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"model":"allowed"}`))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", "application/json")
		if ws {
			r.Header.Set("Upgrade", "websocket")
			r.Header.Set("Connection", "Upgrade")
		}
		w := httptest.NewRecorder()
		p.reverseProxy(w, r, "/v1", up.URL, func(http.Header) {})
		want := 307
		if ws {
			want = 403
		}
		if w.Code != want {
			t.Fatalf("ws=%v status=%d", ws, w.Code)
		}
	}
	if hits.Load() != 1 {
		t.Fatal("unsupported request or redirect reached upstream")
	}
}

func TestModelPolicyConfigIsImmutable(t *testing.T) {
	for _, models := range [][]string{{""}, {"*"}, {"one,two"}, {"bad\n"}} {
		if err := ValidateAllowedModels(models); err == nil {
			t.Fatalf("accepted %q", models)
		}
	}
	models := []string{"allowed"}
	p, err := NewProxy(Config{AllowedModels: models})
	if err != nil {
		t.Fatal(err)
	}
	models[0] = "other"
	if p.cfg.AllowedModels[0] != "allowed" {
		t.Fatal("caller widened policy")
	}
}
