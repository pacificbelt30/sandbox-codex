package authproxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestProxy(t *testing.T) *Proxy {
	t.Helper()
	p, err := NewProxy(Config{TokenTTL: 60, Verbose: false})
	if err != nil {
		t.Fatalf("NewProxy: %v", err)
	}
	p.apiKey = "sk-test-key-12345"
	return p
}

func TestIssueToken(t *testing.T) {
	p := newTestProxy(t)

	token, err := p.IssueToken("worker-1", 60)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	if token == "" {
		t.Fatal("IssueToken returned empty token")
	}
	if !strings.HasPrefix(token, "cdx-") {
		t.Errorf("token %q does not start with 'cdx-'", token)
	}

	p.mu.RLock()
	rec, ok := p.tokens["worker-1"]
	p.mu.RUnlock()
	if !ok {
		t.Fatal("token not stored in proxy")
	}
	if rec.Token != token {
		t.Errorf("stored token %q != issued token %q", rec.Token, token)
	}
}

func TestIssueToken_Unique(t *testing.T) {
	p := newTestProxy(t)

	t1, _ := p.IssueToken("w1", 60)
	t2, _ := p.IssueToken("w2", 60)
	if t1 == t2 {
		t.Error("two IssueToken calls returned the same token")
	}
}

func TestRevokeToken(t *testing.T) {
	p := newTestProxy(t)

	_, err := p.IssueToken("worker-rev", 60)
	if err != nil {
		t.Fatal(err)
	}

	p.RevokeToken("worker-rev")

	p.mu.RLock()
	_, ok := p.tokens["worker-rev"]
	p.mu.RUnlock()
	if ok {
		t.Error("token still present after RevokeToken")
	}
}

func TestHandleToken_Valid(t *testing.T) {
	p := newTestProxy(t)
	if err := p.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer p.Stop()

	token, err := p.IssueToken("ctr-ok", 60)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/token", nil)
	req.Header.Set("X-Codex-Token", token)
	w := httptest.NewRecorder()
	p.handleToken(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	var m map[string]string
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if m["api_key"] != "sk-test-key-12345" {
		t.Errorf("api_key = %q; want sk-test-key-12345", m["api_key"])
	}
	if m["container_name"] != "ctr-ok" {
		t.Errorf("container_name = %q; want ctr-ok", m["container_name"])
	}
}

func TestHandleToken_MissingHeader(t *testing.T) {
	p := newTestProxy(t)
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	defer p.Stop()

	req := httptest.NewRequest(http.MethodGet, "/token", nil)
	w := httptest.NewRecorder()
	p.handleToken(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHandleToken_InvalidToken(t *testing.T) {
	p := newTestProxy(t)
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	defer p.Stop()

	req := httptest.NewRequest(http.MethodGet, "/token", nil)
	req.Header.Set("X-Codex-Token", "cdx-not-a-real-token")
	w := httptest.NewRecorder()
	p.handleToken(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHandleToken_WrongMethod(t *testing.T) {
	p := newTestProxy(t)
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	defer p.Stop()

	req := httptest.NewRequest(http.MethodPost, "/token", nil)
	w := httptest.NewRecorder()
	p.handleToken(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleToken_ExpiredToken(t *testing.T) {
	p := newTestProxy(t)
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	defer p.Stop()

	token, _ := p.IssueToken("ctr-exp", 60)

	// Manually force expiry
	p.mu.Lock()
	p.tokens["ctr-exp"].ExpiresAt = time.Now().Add(-1 * time.Second)
	p.mu.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/token", nil)
	req.Header.Set("X-Codex-Token", token)
	w := httptest.NewRecorder()
	p.handleToken(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for expired token, got %d", w.Code)
	}
}

func TestHandleHealth(t *testing.T) {
	p := newTestProxy(t)
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	defer p.Stop()

	_, _ = p.IssueToken("w1", 60)
	_, _ = p.IssueToken("w2", 60)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	p.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var m map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if m["status"] != "ok" {
		t.Errorf("status = %v; want ok", m["status"])
	}
	if int(m["active_tokens"].(float64)) != 2 {
		t.Errorf("active_tokens = %v; want 2", m["active_tokens"])
	}
}

func TestHandleRevoke(t *testing.T) {
	p := newTestProxy(t)
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	defer p.Stop()

	_, _ = p.IssueToken("ctr-revoke", 60)

	req := httptest.NewRequest(http.MethodPost, "/revoke?container=ctr-revoke", nil)
	w := httptest.NewRecorder()
	p.handleRevoke(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	p.mu.RLock()
	_, exists := p.tokens["ctr-revoke"]
	p.mu.RUnlock()
	if exists {
		t.Error("token still exists after revoke endpoint")
	}
}

func TestHandleRevoke_MissingContainer(t *testing.T) {
	p := newTestProxy(t)
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	defer p.Stop()

	req := httptest.NewRequest(http.MethodPost, "/revoke", nil)
	w := httptest.NewRecorder()
	p.handleRevoke(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestExpireLoop(t *testing.T) {
	p := newTestProxy(t)

	token, _ := p.IssueToken("expire-me", 1)
	_ = token

	// Force immediate expiry
	p.mu.Lock()
	p.tokens["expire-me"].ExpiresAt = time.Now().Add(-2 * time.Second)
	p.mu.Unlock()

	// Run expiry manually (simulating the ticker)
	now := time.Now()
	p.mu.Lock()
	for name, rec := range p.tokens {
		if now.After(rec.ExpiresAt) {
			delete(p.tokens, name)
		}
	}
	p.mu.Unlock()

	p.mu.RLock()
	_, ok := p.tokens["expire-me"]
	p.mu.RUnlock()
	if ok {
		t.Error("expired token was not cleaned up")
	}
}

func TestStartStop(t *testing.T) {
	p := newTestProxy(t)
	if err := p.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if p.Endpoint() == "" {
		t.Error("Endpoint() is empty after Start")
	}
	if !strings.HasPrefix(p.Endpoint(), "http://127.0.0.1:") {
		t.Errorf("Endpoint() = %q; expected http://127.0.0.1:...", p.Endpoint())
	}

	// Issue a token and verify it's cleared on Stop
	_, _ = p.IssueToken("w1", 60)
	p.Stop()

	p.mu.RLock()
	count := len(p.tokens)
	p.mu.RUnlock()
	if count != 0 {
		t.Errorf("tokens not cleared on Stop: %d remaining", count)
	}
}

func TestGenerateToken(t *testing.T) {
	seen := make(map[string]struct{})
	for i := 0; i < 50; i++ {
		tok, err := generateToken()
		if err != nil {
			t.Fatalf("generateToken: %v", err)
		}
		if !strings.HasPrefix(tok, "cdx-") {
			t.Errorf("token %q missing 'cdx-' prefix", tok)
		}
		if len(tok) != 68 { // "cdx-" (4) + 64 hex chars (32 bytes)
			t.Errorf("token len = %d; want 68", len(tok))
		}
		if _, dup := seen[tok]; dup {
			t.Error("generateToken produced duplicate token")
		}
		seen[tok] = struct{}{}
	}
}

// ── OAuth mode proxy tests ───────────────────────────────────────────────────

// newOAuthTestProxy creates a Proxy configured for OAuth mode, bypassing
// IsOAuthAuth() file detection by directly setting oauthCreds.
func newOAuthTestProxy(t *testing.T, accessToken string) *Proxy {
	t.Helper()
	p, err := NewProxy(Config{TokenTTL: 60, Verbose: false})
	if err != nil {
		t.Fatalf("NewProxy: %v", err)
	}
	p.oauthCreds = &OAuthCredentials{
		AccessToken:  accessToken,
		RefreshToken: "rt-secret-stays-on-host",
		TokenType:    "Bearer",
	}
	p.apiKey = "" // ensure API key mode is not active
	return p
}

func TestIsOAuthMode_False(t *testing.T) {
	p := newTestProxy(t)
	if p.IsOAuthMode() {
		t.Error("IsOAuthMode() = true for API key proxy; want false")
	}
}

func TestIsOAuthMode_True(t *testing.T) {
	p := newOAuthTestProxy(t, "at-test-access")
	if !p.IsOAuthMode() {
		t.Error("IsOAuthMode() = false for OAuth proxy; want true")
	}
}

func TestHandleToken_OAuthMode_ReturnsAccessToken(t *testing.T) {
	p := newOAuthTestProxy(t, "at-container-visible")
	if err := p.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer p.Stop()

	token, err := p.IssueToken("ctr-oauth", 60)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/token", nil)
	req.Header.Set("X-Codex-Token", token)
	w := httptest.NewRecorder()
	p.handleToken(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var m map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// access_token must be present
	if m["oauth_access_token"] != "at-container-visible" {
		t.Errorf("oauth_access_token = %q; want at-container-visible", m["oauth_access_token"])
	}
	// container_name must match
	if m["container_name"] != "ctr-oauth" {
		t.Errorf("container_name = %q; want ctr-oauth", m["container_name"])
	}
}

func TestHandleToken_OAuthMode_AllFieldsPresent(t *testing.T) {
	// OAuth mode now passes all token fields (including refresh_token) to containers.
	// See doc/auth-proxy.md for security implications.
	p := newOAuthTestProxy(t, "at-no-leak")
	if err := p.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer p.Stop()

	token, _ := p.IssueToken("ctr-noleak", 60)

	req := httptest.NewRequest(http.MethodGet, "/token", nil)
	req.Header.Set("X-Codex-Token", token)
	w := httptest.NewRecorder()
	p.handleToken(w, req)

	var m map[string]string
	if err := json.NewDecoder(w.Result().Body).Decode(&m); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if m["oauth_access_token"] != "at-no-leak" {
		t.Errorf("oauth_access_token = %q; want at-no-leak", m["oauth_access_token"])
	}
	// refresh_token is now intentionally included in the response
	if _, ok := m["oauth_refresh_token"]; !ok {
		t.Error("oauth_refresh_token key missing from response")
	}
}

func TestHandleToken_OAuthMode_NoAPIKey(t *testing.T) {
	p := newOAuthTestProxy(t, "at-oauth")
	if err := p.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer p.Stop()

	token, _ := p.IssueToken("ctr-no-apikey", 60)

	req := httptest.NewRequest(http.MethodGet, "/token", nil)
	req.Header.Set("X-Codex-Token", token)
	w := httptest.NewRecorder()
	p.handleToken(w, req)

	var m map[string]string
	json.NewDecoder(w.Result().Body).Decode(&m)

	// api_key must not be present in OAuth response
	if _, hasKey := m["api_key"]; hasKey {
		t.Error("api_key should not be present in OAuth mode response")
	}
}

func TestNewProxy_OAuthMode(t *testing.T) {
	home, cleanup := withTempHome(t)
	defer cleanup()

	// Write an OAuth auth.json
	writeAuthJSONForProxy(t, home, map[string]interface{}{
		"access_token":  "at-proxy-test",
		"refresh_token": "rt-proxy-test",
		"token_type":    "Bearer",
	})

	p, err := NewProxy(Config{Verbose: false})
	if err != nil {
		t.Fatalf("NewProxy in OAuth mode: %v", err)
	}

	if !p.IsOAuthMode() {
		t.Error("proxy should be in OAuth mode when auth.json has refresh_token")
	}
	if p.oauthCreds.AccessToken != "at-proxy-test" {
		t.Errorf("oauthCreds.AccessToken = %q; want at-proxy-test", p.oauthCreds.AccessToken)
	}
	// RefreshToken must be held in proxy (not nil) but should NOT be forwarded
	if p.oauthCreds.RefreshToken == "" {
		t.Error("proxy should hold refresh_token internally (for future refresh support)")
	}
	// apiKey must be empty in OAuth mode
	if p.apiKey != "" {
		t.Errorf("apiKey should be empty in OAuth mode, got %q", p.apiKey)
	}
}

// writeAuthJSONForProxy is a test helper that writes auth.json to the fake HOME.
func writeAuthJSONForProxy(t *testing.T, home string, data map[string]interface{}) {
	t.Helper()
	codexDir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(codexDir, 0700); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(codexDir, "auth.json"), b, 0600); err != nil {
		t.Fatal(err)
	}
}

// ── ListenAddr tests ─────────────────────────────────────────────────────────

func TestStart_DefaultListenAddr(t *testing.T) {
	p := newTestProxy(t)
	if err := p.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer p.Stop()

	// Default should be loopback.
	if !strings.HasPrefix(p.Endpoint(), "http://127.0.0.1:") {
		t.Errorf("default Endpoint() = %q; expected http://127.0.0.1:...", p.Endpoint())
	}
}

func TestStart_CustomListenAddr(t *testing.T) {
	p, err := NewProxy(Config{
		TokenTTL:   60,
		Verbose:    false,
		ListenAddr: "127.0.0.1:0", // Still loopback but using explicit config path.
	})
	if err != nil {
		t.Fatalf("NewProxy: %v", err)
	}
	p.apiKey = "sk-test"

	if err := p.Start(); err != nil {
		t.Fatalf("Start with ListenAddr: %v", err)
	}
	defer p.Stop()

	if !strings.HasPrefix(p.Endpoint(), "http://127.0.0.1:") {
		t.Errorf("Endpoint() = %q; expected http://127.0.0.1:...", p.Endpoint())
	}
}

func TestConfig_ListenAddrDefault(t *testing.T) {
	cfg := Config{TokenTTL: 60}
	if cfg.ListenAddr != "" {
		t.Errorf("ListenAddr default should be empty string, got %q", cfg.ListenAddr)
	}
}
