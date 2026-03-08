package authproxy

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// Config configures the Auth Proxy.
type Config struct {
	TokenTTL   int
	Verbose    bool
	ListenAddr string // TCP address to listen on, e.g. "192.168.200.1:0". Defaults to "127.0.0.1:0".
}

// tokenRecord holds a single issued token and its metadata.
type tokenRecord struct {
	Token         string
	ContainerName string
	ExpiresAt     time.Time
	IssuedAt      time.Time
}

// Proxy is the Auth Proxy server that issues short-lived tokens to containers.
type Proxy struct {
	cfg        Config
	apiKey     string
	oauthCreds *OAuthCredentials // non-nil when operating in OAuth mode
	listener   net.Listener
	server     *http.Server
	mu         sync.RWMutex
	tokens     map[string]*tokenRecord // containerName -> record
	addr       string

	// Upstream endpoints; overridable for testing.
	httpClient    *http.Client
	oauthTokenURL string // default: "https://auth.openai.com/oauth/token"
	apiUpstreamURL string // default: "https://api.openai.com/v1" (API key mode)
	chatgptURL    string // default: "https://chatgpt.com/backend-api" (OAuth mode + /chatgpt/)
}

// NewProxy creates a new Auth Proxy.
// In OAuth mode (detected via IsOAuthAuth), credentials are loaded from
// ~/.codex/auth.json. In API key mode, the API key is loaded as usual.
func NewProxy(cfg Config) (*Proxy, error) {
	p := &Proxy{
		cfg:            cfg,
		tokens:         make(map[string]*tokenRecord),
		httpClient:     http.DefaultClient,
		oauthTokenURL:  "https://auth.openai.com/oauth/token",
		apiUpstreamURL: "https://api.openai.com/v1",
		chatgptURL:     "https://chatgpt.com/backend-api",
	}

	if IsOAuthAuth() {
		creds, err := LoadOAuthCredentials()
		if err != nil {
			return nil, fmt.Errorf("loading OAuth credentials: %w", err)
		}
		p.oauthCreds = creds
		if cfg.Verbose {
			fmt.Fprintln(os.Stderr, "Auth Proxy: OAuth mode (access_token only will be issued to containers)")
		}
	} else {
		apiKey := loadAPIKey()
		if apiKey == "" && cfg.Verbose {
			fmt.Fprintln(os.Stderr, "warning: no API key found; containers will not receive auth tokens")
		}
		p.apiKey = apiKey
	}

	return p, nil
}

// IsOAuthMode returns true when the proxy is operating in OAuth mode.
func (p *Proxy) IsOAuthMode() bool {
	return p.oauthCreds != nil
}

// Start begins listening on a random port on the configured address.
// If ListenAddr is empty it defaults to "127.0.0.1:0" (loopback only).
// Pass the dock-net gateway address (e.g. "192.168.200.1:0") so containers
// can reach the proxy over the bridge network (F-NET-04).
func (p *Proxy) Start() error {
	addr := p.cfg.ListenAddr
	if addr == "" {
		addr = "127.0.0.1:0"
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("starting auth proxy: %w", err)
	}
	p.listener = ln
	p.addr = ln.Addr().String()

	mux := http.NewServeMux()
	mux.HandleFunc("/token", p.handleToken)
	mux.HandleFunc("/health", p.handleHealth)
	mux.HandleFunc("/revoke", p.handleRevoke)
	// OAuth token refresh: Codex CLI calls this via CODEX_REFRESH_TOKEN_URL_OVERRIDE.
	// The proxy substitutes the host's real refresh_token so it never reaches containers.
	mux.HandleFunc("/oauth/token", p.handleOAuthTokenRefresh)
	// Responses API reverse proxy: containers set OPENAI_BASE_URL=http://proxy/v1.
	// Forwards to api.openai.com/v1 (API key mode) or chatgpt.com/backend-api/codex (OAuth mode).
	mux.HandleFunc("/v1/", p.handleAPIProxy)
	// ChatGPT backend-api reverse proxy: containers use chatgpt_base_url=http://proxy/chatgpt/.
	mux.HandleFunc("/chatgpt/", p.handleChatGPTProxy)

	p.server = &http.Server{Handler: mux}
	go func() {
		if err := p.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("auth proxy error: %v", err)
		}
	}()

	// Background goroutine to expire tokens
	go p.expireLoop()

	if p.cfg.Verbose {
		fmt.Printf("Auth Proxy listening on %s\n", p.addr)
	}
	return nil
}

// Stop shuts down the proxy and revokes all tokens.
func (p *Proxy) Stop() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if p.server != nil {
		_ = p.server.Shutdown(ctx)
	}
	p.mu.Lock()
	p.tokens = make(map[string]*tokenRecord)
	p.mu.Unlock()
}

// Endpoint returns the proxy URL reachable from containers on dock-net.
func (p *Proxy) Endpoint() string {
	return "http://" + p.addr
}

// IssueToken creates and stores a short-lived token for a container.
func (p *Proxy) IssueToken(containerName string, ttlSec int) (string, error) {
	token, err := generateToken()
	if err != nil {
		return "", err
	}

	rec := &tokenRecord{
		Token:         token,
		ContainerName: containerName,
		IssuedAt:      time.Now(),
		ExpiresAt:     time.Now().Add(time.Duration(ttlSec) * time.Second),
	}

	p.mu.Lock()
	p.tokens[containerName] = rec
	p.mu.Unlock()

	if p.cfg.Verbose {
		fmt.Printf("Token issued for %s (TTL=%ds)\n", containerName, ttlSec)
	}
	return token, nil
}

// RevokeToken immediately revokes the token for a container.
func (p *Proxy) RevokeToken(containerName string) {
	p.mu.Lock()
	delete(p.tokens, containerName)
	p.mu.Unlock()
	if p.cfg.Verbose {
		fmt.Printf("Token revoked for %s\n", containerName)
	}
}

// handleToken responds to a container requesting the API key (verified by token).
func (p *Proxy) handleToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	token := r.Header.Get("X-Codex-Token")
	if token == "" {
		http.Error(w, "missing X-Codex-Token header", http.StatusUnauthorized)
		return
	}

	p.mu.RLock()
	var found *tokenRecord
	for _, rec := range p.tokens {
		if rec.Token == token {
			found = rec
			break
		}
	}
	p.mu.RUnlock()

	if found == nil || time.Now().After(found.ExpiresAt) {
		http.Error(w, "token invalid or expired", http.StatusUnauthorized)
		return
	}

	if p.cfg.Verbose {
		fmt.Printf("Credentials served to container %s\n", found.ContainerName)
	}

	w.Header().Set("Content-Type", "application/json")
	if p.oauthCreds != nil {
		// OAuth mode: pass only access_token and id_token to the container.
		// refresh_token is intentionally withheld; the container must use
		// /oauth/token (CODEX_REFRESH_TOKEN_URL_OVERRIDE) to refresh via the proxy.
		p.mu.RLock()
		creds := *p.oauthCreds
		p.mu.RUnlock()
		_ = json.NewEncoder(w).Encode(map[string]string{
			"oauth_access_token": creds.AccessToken,
			"oauth_id_token":     creds.IDToken,
			"oauth_account_id":   creds.AccountID,
			"oauth_last_refresh": creds.LastRefresh,
			"container_name":     found.ContainerName,
			// oauth_refresh_token intentionally omitted
		})
	} else {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"api_key":        p.apiKey,
			"container_name": found.ContainerName,
		})
	}
}

func (p *Proxy) handleHealth(w http.ResponseWriter, r *http.Request) {
	p.mu.RLock()
	count := len(p.tokens)
	p.mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":        "ok",
		"active_tokens": count,
	})
}

func (p *Proxy) handleRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	containerName := r.URL.Query().Get("container")
	if containerName == "" {
		http.Error(w, "missing container param", http.StatusBadRequest)
		return
	}
	p.RevokeToken(containerName)
	w.WriteHeader(http.StatusOK)
}

func (p *Proxy) expireLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		p.mu.Lock()
		for name, rec := range p.tokens {
			if now.After(rec.ExpiresAt) {
				delete(p.tokens, name)
				if p.cfg.Verbose {
					fmt.Printf("Token expired for %s\n", name)
				}
			}
		}
		p.mu.Unlock()
	}
}

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
	var found *tokenRecord
	for _, rec := range p.tokens {
		if rec.Token == cdxToken {
			found = rec
			break
		}
	}
	p.mu.RUnlock()

	if found == nil || time.Now().After(found.ExpiresAt) {
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
	defer resp.Body.Close()

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

	// Update the host's cached credentials with the new tokens.
	p.mu.Lock()
	if newAccess, ok := tokenResp["access_token"].(string); ok && newAccess != "" {
		p.oauthCreds.AccessToken = newAccess
	}
	if newID, ok := tokenResp["id_token"].(string); ok && newID != "" {
		p.oauthCreds.IDToken = newID
	}
	// Rotate the host's refresh_token if the server issued a new one (RFC 6749 §6).
	if newRefresh, ok := tokenResp["refresh_token"].(string); ok && newRefresh != "" {
		p.oauthCreds.RefreshToken = newRefresh
	}
	p.mu.Unlock()

	// Strip refresh_token before returning to the container.
	delete(tokenResp, "refresh_token")

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
	p.reverseProxy(w, r, "/v1", base)
}

// handleChatGPTProxy proxies /chatgpt/* to https://chatgpt.com/backend-api/*.
// Containers in ChatGPT auth mode set chatgpt_base_url=http://<proxy>/chatgpt/
// in their Codex CLI config so backend-api calls (rate limits, account info, etc.)
// flow through the proxy.
func (p *Proxy) handleChatGPTProxy(w http.ResponseWriter, r *http.Request) {
	p.reverseProxy(w, r, "/chatgpt", p.chatgptURL)
}

// reverseProxy strips stripPrefix from r.URL.Path, appends it to targetBase,
// and forwards the request upstream, copying status, headers, and body back.
func (p *Proxy) reverseProxy(w http.ResponseWriter, r *http.Request, stripPrefix, targetBase string) {
	path := strings.TrimPrefix(r.URL.Path, stripPrefix)
	target := targetBase + path
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "reading request body", http.StatusBadRequest)
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), r.Method, target, bytes.NewReader(body))
	if err != nil {
		http.Error(w, "creating upstream request", http.StatusInternalServerError)
		return
	}

	// Copy headers, skipping hop-by-hop headers.
	hopByHop := map[string]bool{
		"connection":          true,
		"keep-alive":          true,
		"proxy-authenticate":  true,
		"proxy-authorization": true,
		"te":                  true,
		"trailers":            true,
		"transfer-encoding":   true,
		"upgrade":             true,
	}
	for k, vv := range r.Header {
		if hopByHop[strings.ToLower(k)] {
			continue
		}
		for _, v := range vv {
			req.Header.Add(k, v)
		}
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		http.Error(w, "upstream error: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)

	if p.cfg.Verbose {
		fmt.Printf("Proxied %s %s -> %s (%d)\n", r.Method, r.URL.Path, target, resp.StatusCode)
	}
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "cdx-" + hex.EncodeToString(b), nil
}
