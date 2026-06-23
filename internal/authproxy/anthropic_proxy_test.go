package authproxy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newAnthropicAPIKeyProxy returns a proxy configured for Anthropic API-key mode,
// bypassing on-disk credential loading.
func newAnthropicAPIKeyProxy(t *testing.T, key string) *Proxy {
	t.Helper()
	p, err := NewProxy(Config{TokenTTL: 60})
	if err != nil {
		t.Fatalf("NewProxy: %v", err)
	}
	p.anthropicAPIKey = key
	return p
}

// newAnthropicOAuthProxy returns a proxy configured for Anthropic OAuth mode.
func newAnthropicOAuthProxy(t *testing.T, accessToken string) *Proxy {
	t.Helper()
	p, err := NewProxy(Config{TokenTTL: 60})
	if err != nil {
		t.Fatalf("NewProxy: %v", err)
	}
	p.anthropicOAuth = &AnthropicOAuthCredentials{AccessToken: accessToken}
	return p
}

func TestHandleAnthropicProxy_APIKeyMode_InjectsXAPIKey(t *testing.T) {
	var gotPath, gotXAPIKey, gotAuth, gotVersion string
	upstream := newFakeUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotXAPIKey = r.Header.Get("x-api-key")
		gotAuth = r.Header.Get("Authorization")
		gotVersion = r.Header.Get("anthropic-version")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"type":"message"}`))
	})

	p := newAnthropicAPIKeyProxy(t, "sk-ant-real")
	p.anthropicUpstreamURL = upstream.URL
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	defer p.Stop()

	req := httptest.NewRequest(http.MethodPost, "/anthropic/v1/messages",
		strings.NewReader(`{"model":"claude-3-5-sonnet"}`))
	// Container sends a placeholder key; the proxy must overwrite it.
	req.Header.Set("x-api-key", "cdx-placeholder")
	w := httptest.NewRecorder()
	p.handleAnthropicProxy(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if gotPath != "/v1/messages" {
		t.Errorf("upstream path = %q; want /v1/messages", gotPath)
	}
	if gotXAPIKey != "sk-ant-real" {
		t.Errorf("upstream x-api-key = %q; want sk-ant-real", gotXAPIKey)
	}
	if gotAuth != "" {
		t.Errorf("Authorization should be removed in API-key mode, got %q", gotAuth)
	}
	if gotVersion != defaultAnthropicVersion {
		t.Errorf("anthropic-version = %q; want default %q", gotVersion, defaultAnthropicVersion)
	}
}

func TestHandleAnthropicProxy_OAuthMode_InjectsBearer(t *testing.T) {
	var gotXAPIKey, gotAuth, gotBeta string
	upstream := newFakeUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		gotXAPIKey = r.Header.Get("x-api-key")
		gotAuth = r.Header.Get("Authorization")
		gotBeta = r.Header.Get("anthropic-beta")
		w.WriteHeader(http.StatusOK)
	})

	p := newAnthropicOAuthProxy(t, "oat-real-access")
	p.anthropicUpstreamURL = upstream.URL
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	defer p.Stop()

	req := httptest.NewRequest(http.MethodPost, "/anthropic/v1/messages",
		strings.NewReader(`{}`))
	req.Header.Set("x-api-key", "cdx-placeholder")
	w := httptest.NewRecorder()
	p.handleAnthropicProxy(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if gotAuth != "Bearer oat-real-access" {
		t.Errorf("Authorization = %q; want Bearer oat-real-access", gotAuth)
	}
	if gotXAPIKey != "" {
		t.Errorf("x-api-key should be removed in OAuth mode, got %q", gotXAPIKey)
	}
	if !strings.Contains(gotBeta, anthropicOAuthBetaHeader) {
		t.Errorf("anthropic-beta = %q; want it to contain %q", gotBeta, anthropicOAuthBetaHeader)
	}
}

func TestHandleAnthropicProxy_QueryPreserved(t *testing.T) {
	var gotQuery string
	upstream := newFakeUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
	})

	p := newAnthropicAPIKeyProxy(t, "sk-ant")
	p.anthropicUpstreamURL = upstream.URL
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	defer p.Stop()

	req := httptest.NewRequest(http.MethodGet, "/anthropic/v1/models?limit=5", nil)
	w := httptest.NewRecorder()
	p.handleAnthropicProxy(w, req)

	if gotQuery != "limit=5" {
		t.Errorf("upstream query = %q; want limit=5", gotQuery)
	}
}

func TestInjectAnthropicCredentials_PreservesClientVersion(t *testing.T) {
	p := newAnthropicAPIKeyProxy(t, "sk-ant")
	h := http.Header{}
	h.Set("anthropic-version", "2024-10-22")
	p.injectAnthropicCredentials(h)
	if got := h.Get("anthropic-version"); got != "2024-10-22" {
		t.Errorf("anthropic-version = %q; want client value 2024-10-22 preserved", got)
	}
}

func TestMergeBetaHeader(t *testing.T) {
	tests := []struct {
		name     string
		existing string
		want     string
	}{
		{"empty", "", anthropicOAuthBetaHeader},
		{"already present", anthropicOAuthBetaHeader, anthropicOAuthBetaHeader},
		{"appended", "fine-grained-tool-streaming-2025-05-14", "fine-grained-tool-streaming-2025-05-14," + anthropicOAuthBetaHeader},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := http.Header{}
			if tt.existing != "" {
				h.Set("anthropic-beta", tt.existing)
			}
			mergeBetaHeader(h, anthropicOAuthBetaHeader)
			if got := h.Get("anthropic-beta"); got != tt.want {
				t.Errorf("anthropic-beta = %q; want %q", got, tt.want)
			}
		})
	}
}

func TestRefreshAnthropicOAuthIfNeeded_RefreshesExpired(t *testing.T) {
	var gotGrant, gotRefresh string
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotGrant = body["grant_type"]
		gotRefresh = body["refresh_token"]
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"oat-new","refresh_token":"rt-new","expires_in":3600}`))
	}))
	defer fake.Close()

	p := newAnthropicOAuthProxy(t, "oat-old")
	p.anthropicOAuth.RefreshToken = "rt-old"
	p.anthropicOAuth.ExpiresAt = time.Now().UnixMilli() - 1000 // expired
	p.anthropicOAuthTokenURL = fake.URL

	p.refreshAnthropicOAuthIfNeeded(context.Background())

	if gotGrant != "refresh_token" {
		t.Errorf("grant_type sent = %q; want refresh_token", gotGrant)
	}
	if gotRefresh != "rt-old" {
		t.Errorf("refresh_token sent = %q; want rt-old", gotRefresh)
	}
	if p.anthropicOAuth.AccessToken != "oat-new" {
		t.Errorf("AccessToken after refresh = %q; want oat-new", p.anthropicOAuth.AccessToken)
	}
	if p.anthropicOAuth.RefreshToken != "rt-new" {
		t.Errorf("RefreshToken after refresh = %q; want rt-new (rotated)", p.anthropicOAuth.RefreshToken)
	}
	if p.anthropicOAuth.ExpiresAt <= time.Now().UnixMilli() {
		t.Errorf("ExpiresAt should be in the future after refresh, got %d", p.anthropicOAuth.ExpiresAt)
	}
}

func TestRefreshAnthropicOAuthIfNeeded_SkipsWhenValid(t *testing.T) {
	called := false
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer fake.Close()

	p := newAnthropicOAuthProxy(t, "oat-valid")
	p.anthropicOAuth.RefreshToken = "rt"
	p.anthropicOAuth.ExpiresAt = time.Now().UnixMilli() + 3600_000 // 1h out
	p.anthropicOAuthTokenURL = fake.URL

	p.refreshAnthropicOAuthIfNeeded(context.Background())

	if called {
		t.Error("refresh endpoint should not be called when token is still valid")
	}
	if p.anthropicOAuth.AccessToken != "oat-valid" {
		t.Errorf("AccessToken changed unexpectedly: %q", p.anthropicOAuth.AccessToken)
	}
}

func TestHandleAdminMode_AnthropicFields(t *testing.T) {
	p := newAnthropicAPIKeyProxy(t, "sk-ant")
	req := httptest.NewRequest(http.MethodGet, "/admin/mode", nil)
	w := httptest.NewRecorder()
	p.handleAdminMode(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var out map[string]bool
	if err := json.NewDecoder(w.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if !out["anthropic_available"] {
		t.Error("anthropic_available = false; want true")
	}
	if out["anthropic_oauth_mode"] {
		t.Error("anthropic_oauth_mode = true; want false in API-key mode")
	}
}
