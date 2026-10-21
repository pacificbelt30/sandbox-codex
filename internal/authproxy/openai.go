package authproxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// handleOAuthTokenRefresh acts as an OAuth token refresh proxy for containers.
// Codex CLI calls this endpoint via CODEX_REFRESH_TOKEN_URL_OVERRIDE.
// The container authenticates via ?cdx=<short-lived-token> query param.
// The proxy substitutes the host's real refresh_token, calls OpenAI, and returns
// the new access_token WITHOUT the refresh_token (which stays on the host only).
//
// Codex CLI sends JSON:
//
//	{"client_id":"app_EMoamEEZ73f0CkXaXp7hrann","grant_type":"refresh_token","refresh_token":""}
//
// The proxy replaces only the "refresh_token" field; all other fields (grant_type,
// client_id, etc.) are passed through as-is from what Codex CLI sent.
//
// Monitored fields:
//   - request  body["refresh_token"]: replaced with host's real refresh_token
//   - response body["refresh_token"]: stripped before returning to container
func (p *Proxy) handleOAuthTokenRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Authenticate the container via the short-lived token embedded in the URL.
	// Codex CLI does not add custom headers when calling the refresh endpoint,
	// so the token is passed as a query param when setting CODEX_REFRESH_TOKEN_URL_OVERRIDE.
	cdxToken := r.URL.Query().Get("cdx")
	if cdxToken == "" {
		http.Error(w, "missing cdx param", http.StatusUnauthorized)
		return
	}

	p.mu.RLock()
	found := p.tokensByValue[cdxToken]
	p.mu.RUnlock()

	if found == nil || time.Now().After(found.ExpiresAt) || found.Identity.Provider == "anthropic" {
		http.Error(w, "token invalid or expired", http.StatusUnauthorized)
		return
	}
	p.refreshMu.Lock()
	defer p.refreshMu.Unlock()
	// A refresh queued behind another request must recheck revocation.
	p.mu.RLock()
	current := p.tokens[found.ContainerName]
	p.mu.RUnlock()
	if current == nil || current.Token != cdxToken || time.Now().After(current.ExpiresAt) {
		http.Error(w, "token invalid or expired", http.StatusUnauthorized)
		return
	}

	p.mu.RLock()
	if p.oauthCreds == nil {
		p.mu.RUnlock()
		http.Error(w, "not in OAuth mode", http.StatusBadRequest)
		return
	}
	refreshToken := p.oauthCreds.RefreshToken
	p.mu.RUnlock()

	// Parse the JSON body that Codex CLI sent.
	// Only "refresh_token" is replaced; all other fields (grant_type, client_id, …)
	// are kept as-is. The container's auth.json has refresh_token="" so Codex CLI
	// sends an empty string here; the proxy substitutes the host's real token.
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "reading request body", http.StatusBadRequest)
		return
	}
	var reqBody map[string]interface{}
	if len(body) > 0 {
		if err := json.Unmarshal(body, &reqBody); err != nil {
			http.Error(w, "parsing request body", http.StatusBadRequest)
			return
		}
	} else {
		reqBody = map[string]interface{}{}
	}
	// Replace only the refresh_token; leave grant_type, client_id, etc. unchanged.
	reqBody["refresh_token"] = refreshToken

	newBody, err := json.Marshal(reqBody)
	if err != nil {
		http.Error(w, "encoding request body", http.StatusInternalServerError)
		return
	}

	// Forward to the real OpenAI OAuth endpoint with Content-Type: application/json.
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, p.oauthTokenURL,
		bytes.NewReader(newBody))
	if err != nil {
		http.Error(w, "creating refresh request", http.StatusInternalServerError)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		http.Error(w, "calling OAuth endpoint: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "reading OAuth response", http.StatusInternalServerError)
		return
	}

	if resp.StatusCode != http.StatusOK {
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(respBody)
		return
	}

	var tokenResp map[string]interface{}
	if err := json.Unmarshal(respBody, &tokenResp); err != nil {
		http.Error(w, "parsing OAuth response", http.StatusInternalServerError)
		return
	}

	// Persist the next generation before exposing it to other requests.
	p.mu.RLock()
	next := *p.oauthCreds
	p.mu.RUnlock()
	if newAccess, ok := tokenResp["access_token"].(string); ok && newAccess != "" {
		next.AccessToken = newAccess
	}
	if newID, ok := tokenResp["id_token"].(string); ok && newID != "" {
		next.IDToken = newID
	}
	// Rotate the host's refresh_token if the server issued a new one (RFC 6749 §6).
	if newRefresh, ok := tokenResp["refresh_token"].(string); ok && newRefresh != "" {
		next.RefreshToken = newRefresh
	}
	if err := p.persistOAuthState("openai", &next); err != nil {
		http.Error(w, "saving OAuth refresh failed", http.StatusBadGateway)
		return
	}
	p.mu.Lock()
	*p.oauthCreds = next
	p.mu.Unlock()

	// Strip refresh_token before returning to the container.
	delete(tokenResp, "refresh_token")

	// Replace real access_token with the container's placeholder token so the
	// real credential never reaches the container.  The proxy injects the real
	// access_token on every outbound API request via injectCredentials.
	if _, ok := tokenResp["access_token"].(string); ok {
		tokenResp["access_token"] = found.Token
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(tokenResp)

	if p.cfg.Verbose {
		fmt.Printf("OAuth token refreshed for container %s\n", found.ContainerName)
	}
}

// handleAPIProxy proxies /v1/* to the real Responses API backend.
// In API key mode: forwards to https://api.openai.com/v1/
// In OAuth/ChatGPT mode: forwards to https://chatgpt.com/backend-api/codex/
//
// Containers should set OPENAI_BASE_URL=http://<proxy>/v1 so Codex CLI routes
// all Responses API traffic through the proxy.
func (p *Proxy) handleAPIProxy(w http.ResponseWriter, r *http.Request) {
	var base string
	if p.oauthCreds != nil {
		base = p.chatgptURL + "/codex"
	} else {
		base = p.apiUpstreamURL
	}
	p.reverseProxy(w, r, "/v1", base, p.injectCredentials)
}

// handleChatGPTProxy proxies /chatgpt/* to https://chatgpt.com/backend-api/*.
// Containers in ChatGPT auth mode set chatgpt_base_url=http://<proxy>/chatgpt/
// in their Codex CLI config so backend-api calls (rate limits, account info, etc.)
// flow through the proxy.
func (p *Proxy) handleChatGPTProxy(w http.ResponseWriter, r *http.Request) {
	p.reverseProxy(w, r, "/chatgpt", p.chatgptURL, p.injectCredentials)
}

// injectCredentials overwrites Authorization (and ChatGPT-Account-Id in OAuth mode)
// in h with the real host credentials, replacing any placeholder value the container sent.
// Safe to call concurrently.
func (p *Proxy) injectCredentials(h http.Header) {
	if p.oauthCreds != nil {
		p.mu.RLock()
		accessToken := p.oauthCreds.AccessToken
		accountID := p.oauthCreds.AccountID
		p.mu.RUnlock()
		h.Set("Authorization", "Bearer "+accessToken)
		if accountID != "" {
			h.Set("ChatGPT-Account-Id", accountID)
		}
		return
	}
	if p.apiKey != "" {
		h.Set("Authorization", "Bearer "+p.apiKey)
	}
}
