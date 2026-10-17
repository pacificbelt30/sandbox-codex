package authproxy

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

func TestRestoreTokenRequiresAdminAndStaysOffDataPlane(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-fixture-only")
	p, err := NewProxy(Config{Role: RoleAuth, TokenTTL: 60, ListenAddr: "127.0.0.1:0", AdminListenAddr: "127.0.0.1:0", AdminSecret: "fixture-admin"})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Stop)
	token, err := p.IssueToken("worker", 60)
	if err != nil {
		t.Fatal(err)
	}
	p.RevokeToken("worker")
	body, _ := json.Marshal(map[string]any{"container": "worker", "token": token, "ttl": 60})
	post := func(endpoint, secret string) int {
		t.Helper()
		req, _ := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
		req.Header.Set("X-Proxy-Admin-Secret", secret)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		return resp.StatusCode
	}
	if code := post("http://"+p.adminAddr+"/admin/restore", ""); code != http.StatusUnauthorized {
		t.Fatalf("unauthorized restore: %d", code)
	}
	if code := post(p.Endpoint()+"/admin/restore", "fixture-admin"); code == http.StatusNoContent {
		t.Fatal("restore exposed on worker data plane")
	}
	r, err := NewRemoteProxy("http://"+p.adminAddr, p.Endpoint(), "fixture-admin")
	if err != nil {
		t.Fatal(err)
	}
	if err := r.RestoreToken("worker", token, 60); err != nil {
		t.Fatal(err)
	}
	p.mu.Lock()
	rec := p.tokens["worker"]
	p.mu.Unlock()
	if rec == nil || rec.Token != token {
		t.Fatal("retained worker token not restored")
	}
	if err := p.RestoreToken("other-worker", token, 60); err == nil {
		t.Fatal("token rebound to a different active worker")
	}
	for _, bad := range []string{"", "sk-actual-secret", "cdx-bad", "cdx-" + string(bytes.Repeat([]byte{'x'}, 64))} {
		if err := p.RestoreToken("worker", bad, 60); err == nil {
			t.Fatal("invalid placeholder accepted")
		}
	}
	p.RevokeToken("worker")
	p.mu.Lock()
	_, exists := p.tokens["worker"]
	p.mu.Unlock()
	if exists {
		t.Fatal("restored token did not revoke")
	}
}
