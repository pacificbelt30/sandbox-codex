package authproxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestOAuthStateRestoresRotationOnlyForSameLogin(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	p := &Proxy{cfg: Config{OAuthStateDir: dir}, oauthSource: map[string]string{"openai": oauthSourceHash("login-refresh")}}
	rotated := &OAuthCredentials{AccessToken: "new-access", RefreshToken: "new-refresh"}
	if err := p.persistOAuthState("openai", rotated); err != nil {
		t.Fatal(err)
	}
	var restored OAuthCredentials
	if err := p.restoreOAuthState("openai", "login-refresh", &restored); err != nil {
		t.Fatal(err)
	}
	if restored != *rotated {
		t.Fatalf("restored %+v", restored)
	}
	other := OAuthCredentials{AccessToken: "other-login", RefreshToken: "other-refresh"}
	if err := p.restoreOAuthState("openai", "other-refresh", &other); err != nil {
		t.Fatal(err)
	}
	if other.AccessToken != "other-login" {
		t.Fatal("stale refresh crossed login boundary")
	}
}

func TestConcurrentRefreshUsesRotatedToken(t *testing.T) {
	var mu sync.Mutex
	var sent []string
	fake := fakeOAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		var request map[string]string
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		mu.Lock()
		sent = append(sent, request["refresh_token"])
		index := len(sent)
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "access", "refresh_token": "rotated-" + string(rune('0'+index))})
	})
	p := newOAuthProxyWithFakeOAuth(t, fake)
	p.cfg.OAuthStateDir = filepath.Join(t.TempDir(), "state")
	p.oauthSource = map[string]string{"openai": oauthSourceHash(p.oauthCreds.RefreshToken)}
	cdx, _ := p.IssueToken("worker", 60)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := httptest.NewRequest(http.MethodPost, "/oauth/token?cdx="+cdx, strings.NewReader(`{"refresh_token":""}`))
			w := httptest.NewRecorder()
			p.handleOAuthTokenRefresh(w, r)
			if w.Code != http.StatusOK {
				t.Errorf("refresh status %d: %s", w.Code, w.Body.String())
			}
		}()
	}
	wg.Wait()
	if len(sent) != 2 || sent[0] != "rt-secret-stays-on-host" || sent[1] != "rotated-1" {
		t.Fatalf("refresh sequence %v", sent)
	}
}
