package authproxy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/pacificbelt30/codex-dock/pkg/cxd/credentialstore"
)

func TestIndependentProfilesRequireExclusiveProtectedStore(t *testing.T) {
	store, err := credentialstore.NewFileStore(filepath.Join(t.TempDir(), "profile"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(context.Background(), "work", credentialstore.Credentials{OpenAIAPIKey: "fixture-key"}); err != nil {
		t.Fatal(err)
	}
	base := Config{CredentialStore: store, ProfileName: "work", AdminSecret: strings.Repeat("a", 32), AdminListenAddr: "127.0.0.1:0"}
	for _, change := range []func(*Config){
		func(c *Config) { c.CredentialStore = nil },
		func(c *Config) { c.CaseID = "external" },
		func(c *Config) { c.AdminSecret = "" },
		func(c *Config) { c.AdminListenAddr = "" },
		func(c *Config) { c.Role = RoleEgress },
		func(c *Config) { c.BrokerOnly = true },
		func(c *Config) { c.OAuthStateDir = t.TempDir() },
	} {
		cfg := base
		change(&cfg)
		if _, err := NewProxy(cfg); err == nil {
			t.Fatal("unsafe profile configuration accepted")
		}
	}
}

func TestIndependentProfilesProxyFullPath(t *testing.T) {
	for _, mode := range []string{"openai-key", "openai-oauth", "anthropic-key", "anthropic-oauth"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("OPENAI_API_KEY", "fixture-host-key-not-selected")
			t.Setenv("ANTHROPIC_API_KEY", "fixture-host-anthropic-not-selected")
			t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "fixture-host-oauth-not-selected")
			t.Setenv("HOME", t.TempDir())
			if err := os.Mkdir(filepath.Join(os.Getenv("HOME"), ".codex"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(os.Getenv("HOME"), ".codex/auth.json"), []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"host-access","refresh_token":"host-refresh"}}`), 0600); err != nil {
				t.Fatal(err)
			}
			var credentials credentialstore.Credentials
			switch mode {
			case "openai-key":
				credentials.OpenAIAPIKey = "fixture-profile-key"
			case "openai-oauth":
				credentials.OpenAIOAuth = &credentialstore.OpenAIOAuth{AccessToken: "fixture-profile-access", RefreshToken: "fixture-profile-refresh", IDToken: privacyIDToken(t), AccountID: "acct-profile"}
			case "anthropic-key":
				credentials.AnthropicAPIKey = "fixture-profile-key"
			case "anthropic-oauth":
				credentials.AnthropicOAuth = &credentialstore.AnthropicOAuth{AccessToken: "fixture-profile-access", RefreshToken: "fixture-profile-refresh"}
			}
			store, err := credentialstore.NewFileStore(filepath.Join(t.TempDir(), "profile"))
			if err != nil {
				t.Fatal(err)
			}
			if err := store.Create(context.Background(), mode, credentials); err != nil {
				t.Fatal(err)
			}
			p, err := NewProxy(Config{CredentialStore: store, ProfileName: mode, AdminSecret: strings.Repeat("a", 32), AdminListenAddr: "127.0.0.1:0"})
			if err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int32
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if strings.HasPrefix(mode, "anthropic") && strings.HasSuffix(mode, "key") {
					if r.Header.Get("X-Api-Key") != "fixture-profile-key" || r.Header.Get("Authorization") != "" {
						t.Error("wrong Anthropic profile injection")
					}
				} else {
					want := "fixture-profile-key"
					if strings.HasSuffix(mode, "oauth") {
						want = "fixture-profile-access"
					}
					if r.Header.Get("Authorization") != "Bearer "+want {
						t.Error("host credentials or placeholder replaced the selected profile")
					}
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			defer up.Close()
			p.apiUpstreamURL, p.chatgptURL, p.anthropicUpstreamURL = up.URL, up.URL, up.URL
			if err := p.Start(); err != nil {
				t.Fatal(err)
			}
			defer p.Stop()
			remote, err := NewRemoteProxy("http://"+p.adminAddr, p.Endpoint(), strings.Repeat("a", 32))
			if err != nil || remote.AuthProfile() != mode || remote.IsCaseProxy() || remote.ProfileConnectionVersion() == "" {
				t.Fatal("independent profile capability missing")
			}
			token, err := remote.IssueToken("worker", 60)
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodGet, "/token", nil)
			request.Header.Set("X-Codex-Token", token)
			result := httptest.NewRecorder()
			p.handleToken(result, request)
			assertPrivateTokensAbsent(t, result.Body.String(), "fixture-profile-key", "fixture-profile-access", "fixture-profile-refresh", "host-access", "host-refresh", "private-original-signature")
			path := "/v1/responses"
			if strings.HasPrefix(mode, "anthropic") {
				path = "/anthropic/v1/messages"
			}
			do := func() int {
				req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, p.Endpoint()+path, strings.NewReader(`{"model":"fixture"}`))
				if err != nil {
					t.Fatal(err)
				}
				req.Header.Set("Authorization", "Bearer "+token)
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
				return resp.StatusCode
			}
			if do() != http.StatusNoContent || calls.Load() != 1 {
				t.Fatal("profile request did not reach its upstream")
			}
			if err := remote.RevokeTokenChecked("worker"); err != nil {
				t.Fatal(err)
			}
			if do() != http.StatusUnauthorized || calls.Load() != 1 {
				t.Fatal("revoked profile token reached upstream")
			}
			if _, err := NewProxy(Config{CredentialStore: store, ProfileName: "missing", AdminSecret: strings.Repeat("a", 32), AdminListenAddr: "127.0.0.1:0"}); err == nil {
				t.Fatal("missing profile fell back to host credentials")
			}
		})
	}
}
