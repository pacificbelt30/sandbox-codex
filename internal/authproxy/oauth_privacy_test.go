package authproxy

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type privacyTransport func(*http.Request) (*http.Response, error)

func (f privacyTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func privacyIDToken(t *testing.T) string {
	t.Helper()
	payload := `{"email":"private-email@example.invalid","nonce":"private-nonce","https://api.openai.com/auth":{"chatgpt_account_id":"acct-privacy","chatgpt_plan_type":"enterprise","chatgpt_account_is_fedramp":true,"chatgpt_user_id":"private-user","future_token":"private-future-token"}}`
	return base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","kid":"private-key-id"}`)) + "." + base64.RawURLEncoding.EncodeToString([]byte(payload)) + ".private-original-signature"
}

func assertPrivateTokensAbsent(t *testing.T, data string, secrets ...string) {
	t.Helper()
	for _, secret := range secrets {
		if secret != "" && strings.Contains(data, secret) {
			t.Fatal("host credential or unselected private claim crossed the worker boundary")
		}
	}
}

func assertWorkerIDToken(t *testing.T, token string) {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		t.Fatal("worker ID metadata must be readable by the pinned CLI JWT parser")
	}
	header, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || string(header) != `{"alg":"none","typ":"JWT"}` {
		t.Fatal("worker ID metadata must have a fresh unsigned header")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	assertPrivateTokensAbsent(t, string(payload), "private-email", "private-nonce", "private-user", "private-future-token")
	var claims map[string]map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatal(err)
	}
	auth := claims["https://api.openai.com/auth"]
	if len(claims) != 1 || len(auth) != 3 || auth["chatgpt_account_id"] != "acct-privacy" || auth["chatgpt_plan_type"] != "enterprise" || auth["chatgpt_account_is_fedramp"] != true {
		t.Fatal("worker account/plan metadata changed or extra claims were copied")
	}
}

func TestHandleTokenKeepsRealIDTokenOnHost(t *testing.T) {
	p := newOAuthTestProxy(t, "private-access-token")
	p.oauthCreds.IDToken = privacyIDToken(t)
	p.oauthCreds.AccountID = "acct-privacy"
	token, _ := p.IssueToken("privacy", 60)
	r := httptest.NewRequest(http.MethodGet, "/token", nil)
	r.Header.Set("X-Codex-Token", token)
	w := httptest.NewRecorder()
	p.handleToken(w, r)
	assertPrivateTokensAbsent(t, w.Body.String(), p.oauthCreds.IDToken, p.oauthCreds.AccessToken, p.oauthCreds.RefreshToken, "private-original-signature")
	var response map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	assertWorkerIDToken(t, response["oauth_id_token"])
	if response["oauth_access_token"] != token {
		t.Fatal("worker did not receive its access placeholder")
	}
}

func TestRefreshReturnsOnlyWorkerTokensAndRetainsHostRotation(t *testing.T) {
	id := privacyIDToken(t)
	fake := fakeOAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "private-new-access", "id_token": id, "refresh_token": "private-new-refresh",
			"future_credential": "private-future-token", "nested": map[string]string{"token": "private-nested-token"},
		})
	})
	p := newOAuthProxyWithFakeOAuth(t, fake)
	p.oauthCreds.AccountID = "acct-privacy"
	token, _ := p.IssueToken("privacy", 60)
	w := httptest.NewRecorder()
	p.handleOAuthTokenRefresh(w, httptest.NewRequest(http.MethodPost, "/oauth/token?cdx="+token, strings.NewReader(`{}`)))
	if w.Code != http.StatusOK {
		t.Fatalf("refresh status %d", w.Code)
	}
	assertPrivateTokensAbsent(t, w.Body.String(), id, "private-new-access", "private-new-refresh", "private-future-token", "private-nested-token")
	var response map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response) != 2 || response["access_token"] != token {
		t.Fatal("refresh must expose only worker access/ID placeholders")
	}
	assertWorkerIDToken(t, response["id_token"])
	if p.oauthCreds.IDToken != id || p.oauthCreds.AccessToken != "private-new-access" || p.oauthCreds.RefreshToken != "private-new-refresh" {
		t.Fatal("real credential rotation was not retained on the host")
	}
}

func TestRefreshErrorBodyCannotLeakTokens(t *testing.T) {
	for _, body := range []string{
		`{"error":{"code":"refresh_token_reused","message":"private-error-secret"},"id_token":"private-error-secret"}`,
		`{"error":"INVALID_GRANT","error_description":"private-error-secret"}`,
		`{"code":"refresh_token_expired","debug":"private-error-secret"}`,
		`{"error":"private-error-secret"}`, `not JSON private-error-secret`,
	} {
		t.Run(body[:8], func(t *testing.T) {
			fake := fakeOAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = io.WriteString(w, body)
			})
			p := newOAuthProxyWithFakeOAuth(t, fake)
			token, _ := p.IssueToken("privacy", 60)
			w := httptest.NewRecorder()
			p.handleOAuthTokenRefresh(w, httptest.NewRequest(http.MethodPost, "/oauth/token?cdx="+token, nil))
			if w.Code != http.StatusBadRequest {
				t.Fatalf("upstream error status changed: %d", w.Code)
			}
			assertPrivateTokensAbsent(t, w.Body.String(), "private-error-secret")
			for _, code := range []string{"refresh_token_reused", "invalid_grant", "refresh_token_expired"} {
				if strings.Contains(strings.ToLower(body), code) && !strings.Contains(w.Body.String(), code) {
					t.Fatal("pinned CLI refresh error classification was lost")
				}
			}
		})
	}
}

func TestRefreshTransportErrorCannotLeakTokens(t *testing.T) {
	p := newOAuthTestProxy(t, "private-access-token")
	p.httpClient = &http.Client{Transport: privacyTransport(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("private-transport-secret")
	})}
	token, _ := p.IssueToken("privacy", 60)
	w := httptest.NewRecorder()
	p.handleOAuthTokenRefresh(w, httptest.NewRequest(http.MethodPost, "/oauth/token?cdx="+token, nil))
	if w.Code != http.StatusBadGateway {
		t.Fatalf("transport failure status %d", w.Code)
	}
	assertPrivateTokensAbsent(t, w.Body.String(), "private-transport-secret")
}

func TestRefreshMalformedSuccessCannotAlterHostOrExposeIssuerFields(t *testing.T) {
	for _, body := range []string{`null`, `{}`, `{"access_token":{"nested":"private-malformed-secret"}}`, `{"access_token":""}`} {
		t.Run(body, func(t *testing.T) {
			fake := fakeOAuthServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, body) })
			p := newOAuthProxyWithFakeOAuth(t, fake)
			before := *p.oauthCreds
			token, _ := p.IssueToken("privacy", 60)
			w := httptest.NewRecorder()
			p.handleOAuthTokenRefresh(w, httptest.NewRequest(http.MethodPost, "/oauth/token?cdx="+token, nil))
			if w.Code != http.StatusBadGateway || *p.oauthCreds != before {
				t.Fatal("invalid issuer response was published as a successful rotation")
			}
			assertPrivateTokensAbsent(t, w.Body.String(), "private-malformed-secret")
		})
	}
}

func TestWorkerIDMetadataHandlesAbsentAndMalformedHostID(t *testing.T) {
	for _, id := range []string{"", "private-invalid-id", "header.invalid.private-signature", "header.bnVsbA.private-signature"} {
		t.Run(id, func(t *testing.T) {
			token := workerIDToken(OAuthCredentials{IDToken: id, AccountID: "acct-safe"})
			payload, err := base64.RawURLEncoding.DecodeString(strings.Split(token, ".")[1])
			if err != nil {
				t.Fatal(err)
			}
			if string(payload) != `{"https://api.openai.com/auth":{"chatgpt_account_id":"acct-safe"}}` {
				t.Fatal("missing or malformed host ID did not produce minimal worker metadata")
			}
			assertPrivateTokensAbsent(t, token, "private-invalid-id", "private-signature")
		})
	}
}

func TestRefreshErrorClassificationPrecedenceAndCase(t *testing.T) {
	for _, tc := range []struct{ body, code string }{
		{`{"error":"REFRESH_TOKEN_INVALIDATED"}`, "refresh_token_invalidated"},
		{`{"error":{"code":"REFRESH_TOKEN_REUSED"},"code":"invalid_grant"}`, "refresh_token_reused"},
		{`{"error":"unknown","code":"invalid_grant"}`, "oauth_refresh_failed"},
		{`{"error":"","code":"invalid_grant"}`, "invalid_grant"},
	} {
		if got := workerRefreshErrorCode([]byte(tc.body)); got != tc.code {
			t.Fatalf("refresh classification = %s; want %s", got, tc.code)
		}
	}
}
