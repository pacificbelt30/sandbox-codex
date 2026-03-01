package authproxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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
