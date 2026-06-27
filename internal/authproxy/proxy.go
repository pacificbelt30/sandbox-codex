package authproxy

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// Config configures the Auth Proxy.
type Config struct {
	TokenTTL    int
	Verbose     bool
	ListenAddr  string // worker-facing TCP address (data plane + forward proxy). Defaults to "0.0.0.0:0".
	AdminSecret string

	// AdminListenAddr, when non-empty, serves the /admin/* routes on a separate
	// listener so they are not reachable on the worker-facing data-plane port.
	// When empty, /admin/* is served on the main listener (in-process/test use).
	AdminListenAddr string

	// ForwardAllowDomains optionally restricts the HTTP CONNECT forward proxy to
	// the listed domains (and their subdomains). Empty means allow all hosts.
	ForwardAllowDomains []string
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

	// Admin listener (only used when Config.AdminListenAddr is set).
	adminListener net.Listener
	adminServer   *http.Server
	adminAddr     string

	// Anthropic (Claude Code) credentials. Populated independently of the
	// OpenAI credentials above so a single proxy can serve both agents.
	anthropicAPIKey string                     // non-empty in Anthropic API-key mode
	anthropicOAuth  *AnthropicOAuthCredentials // non-nil in Claude subscription (OAuth) mode

	// Upstream endpoints; overridable for testing.
	httpClient     *http.Client
	oauthTokenURL  string // default: "https://auth.openai.com/oauth/token"
	apiUpstreamURL string // default: "https://api.openai.com/v1" (API key mode)
	chatgptURL     string // default: "https://chatgpt.com/backend-api" (OAuth mode + /chatgpt/)

	// Anthropic upstream + OAuth refresh endpoint; overridable for testing.
	anthropicUpstreamURL   string // default: "https://api.anthropic.com"
	anthropicOAuthTokenURL string // default: "https://console.anthropic.com/v1/oauth/token"
	anthropicOAuthClientID string // Claude Code public OAuth client id
}

// AdminBindEgress is a sentinel host for AdminListenAddr meaning "bind the admin
// listener to the container's egress (primary) IPv4 address". This keeps /admin/*
// reachable from the host (via the published port, which DNATs to that IP) while
// making it unreachable from worker containers, which only know the proxy's IP on
// their own Internal network. Falls back to all interfaces if detection fails.
const AdminBindEgress = "egress"

// anthropicOAuthBetaHeader is required by the Anthropic API when authenticating
// with a Claude subscription OAuth bearer token instead of an API key.
const anthropicOAuthBetaHeader = "oauth-2025-04-20"

// defaultAnthropicVersion is sent when the container omits anthropic-version.
const defaultAnthropicVersion = "2023-06-01"

// NewProxy creates a new Auth Proxy.
// In OAuth mode (detected via IsOAuthAuth), credentials are loaded from
// ~/.codex/auth.json. In API key mode, the API key is loaded as usual.
func NewProxy(cfg Config) (*Proxy, error) {
	p := &Proxy{
		cfg:                    cfg,
		tokens:                 make(map[string]*tokenRecord),
		httpClient:             http.DefaultClient,
		oauthTokenURL:          "https://auth.openai.com/oauth/token",
		apiUpstreamURL:         "https://api.openai.com/v1",
		chatgptURL:             "https://chatgpt.com/backend-api",
		anthropicUpstreamURL:   "https://api.anthropic.com",
		anthropicOAuthTokenURL: "https://console.anthropic.com/v1/oauth/token",
		anthropicOAuthClientID: "9d1c250a-e61b-44d9-88ed-5944d1962f5e",
	}

	if IsOAuthAuth() {
		creds, err := LoadOAuthCredentials()
		if err != nil {
			return nil, fmt.Errorf("loading OAuth credentials: %w", err)
		}
		p.oauthCreds = creds
		if cfg.Verbose {
			fmt.Fprintln(os.Stderr, "Auth Proxy: OpenAI OAuth mode (placeholder tokens issued to containers; proxy injects real credentials on outbound requests)")
		}
	} else {
		apiKey := loadAPIKey()
		if apiKey == "" && cfg.Verbose {
			fmt.Fprintln(os.Stderr, "warning: no OpenAI API key found; Codex containers will not receive auth tokens")
		}
		p.apiKey = apiKey
	}

	// Anthropic (Claude Code) credentials are loaded independently so the proxy
	// can serve both agents from a single endpoint.
	if IsAnthropicOAuth() {
		creds, err := LoadAnthropicOAuthCredentials()
		if err != nil {
			return nil, fmt.Errorf("loading Anthropic OAuth credentials: %w", err)
		}
		p.anthropicOAuth = creds
		if cfg.Verbose {
			fmt.Fprintln(os.Stderr, "Auth Proxy: Anthropic OAuth mode (Claude subscription; proxy injects real bearer token on outbound requests)")
		}
	} else if key := loadAnthropicAPIKey(); key != "" {
		p.anthropicAPIKey = key
		if cfg.Verbose {
			fmt.Fprintln(os.Stderr, "Auth Proxy: Anthropic API-key mode (proxy injects real x-api-key on outbound requests)")
		}
	}

	return p, nil
}

// IsAnthropicMode returns true when the proxy can serve Anthropic (Claude Code)
// requests, in either API-key or OAuth mode.
func (p *Proxy) IsAnthropicMode() bool {
	return p.anthropicOAuth != nil || p.anthropicAPIKey != ""
}

// IsAnthropicOAuthMode returns true when the proxy serves Anthropic requests
// using a Claude subscription OAuth token rather than an API key.
func (p *Proxy) IsAnthropicOAuthMode() bool {
	return p.anthropicOAuth != nil
}

// IsOAuthMode returns true when the proxy is operating in OAuth mode.
func (p *Proxy) IsOAuthMode() bool {
	return p.oauthCreds != nil
}

// Start begins listening on a random port on the configured address.
// If ListenAddr is empty it defaults to "0.0.0.0:0" (all interfaces).
// Binding to all interfaces allows worker containers to reach the proxy
// via host.docker.internal (resolves to the Docker bridge gateway IP).
func (p *Proxy) Start() error {
	addr := p.cfg.ListenAddr
	if addr == "" {
		addr = "0.0.0.0:0"
	}
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", addr)
	if err != nil {
		return fmt.Errorf("starting auth proxy: %w", err)
	}
	p.listener = ln
	p.addr = ln.Addr().String()

	// Data-plane mux: routes the worker is allowed to use. The /admin/* routes are
	// registered here only when no separate admin listener is configured.
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
	// Anthropic (Claude Code) reverse proxy: containers set ANTHROPIC_BASE_URL=http://proxy/anthropic.
	// Forwards to api.anthropic.com, injecting the host's real API key or OAuth bearer token.
	mux.HandleFunc("/anthropic/", p.handleAnthropicProxy)

	adminMux := http.NewServeMux()
	adminMux.HandleFunc("/admin/issue", p.handleAdminIssue)
	adminMux.HandleFunc("/admin/revoke", p.handleAdminRevoke)
	adminMux.HandleFunc("/admin/mode", p.handleAdminMode)

	if p.cfg.AdminListenAddr != "" {
		// Serve /admin/* on a dedicated listener so workers cannot reach token
		// issuance. With AdminBindEgress this binds to the egress IP only, so the
		// admin port is unreachable from the per-worker Internal networks.
		adminListen := resolveAdminListenAddr(p.cfg.AdminListenAddr)
		aln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", adminListen)
		if err != nil {
			_ = ln.Close()
			return fmt.Errorf("starting auth proxy admin listener: %w", err)
		}
		p.adminListener = aln
		p.adminAddr = aln.Addr().String()
		p.adminServer = &http.Server{Handler: adminMux}
		go func() {
			if err := p.adminServer.Serve(aln); err != nil && err != http.ErrServerClosed {
				log.Printf("auth proxy admin error: %v", err)
			}
		}()
		if p.cfg.AdminSecret == "" {
			log.Printf("warning: auth proxy admin listener has no --admin-secret; /admin/* is unauthenticated")
		}
	} else {
		// Single-listener mode (in-process/tests): admin routes share the main mux.
		mux.HandleFunc("/admin/issue", p.handleAdminIssue)
		mux.HandleFunc("/admin/revoke", p.handleAdminRevoke)
		mux.HandleFunc("/admin/mode", p.handleAdminMode)
	}

	// Worker-facing handler: CONNECT/absolute-form requests are handled by the
	// forward proxy (router); everything else falls through to the data-plane mux.
	p.server = &http.Server{Handler: p.workerFacingHandler(mux)}
	go func() {
		if err := p.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("auth proxy error: %v", err)
		}
	}()

	// Background goroutine to expire tokens
	go p.expireLoop()

	if p.cfg.Verbose {
		fmt.Printf("Auth Proxy listening on %s\n", p.addr)
		if p.adminAddr != "" {
			fmt.Printf("Auth Proxy admin listening on %s\n", p.adminAddr)
		}
	}
	return nil
}

// resolveAdminListenAddr maps the AdminBindEgress sentinel host to the container's
// egress (primary) IPv4 so the admin port is unreachable from worker Internal
// networks. Any other host is returned unchanged.
func resolveAdminListenAddr(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil || host != AdminBindEgress {
		return addr
	}
	if ip := primaryNonLoopbackIPv4(); ip != "" {
		return net.JoinHostPort(ip, port)
	}
	// Detection failed: fall back to all interfaces so the proxy still starts.
	log.Printf("warning: could not determine egress IP for admin listener; binding all interfaces")
	return net.JoinHostPort("0.0.0.0", port)
}

// primaryNonLoopbackIPv4 returns the first global-unicast IPv4 address on a
// non-loopback interface. At proxy startup the container is attached only to the
// egress network, so this is the egress IP; the per-worker networks are connected
// later by the sandbox manager.
func primaryNonLoopbackIPv4() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipnet.IP.To4()
			if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				continue
			}
			return ip.String()
		}
	}
	return ""
}

func (p *Proxy) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if p.cfg.AdminSecret == "" {
		return true
	}
	if r.Header.Get("X-Proxy-Admin-Secret") != p.cfg.AdminSecret {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return false
	}
	return true
}

func (p *Proxy) handleAdminIssue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !p.requireAdmin(w, r) {
		return
	}
	var req struct {
		Container string `json:"container"`
		TTL       int    `json:"ttl"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.Container == "" {
		http.Error(w, "missing container", http.StatusBadRequest)
		return
	}
	if req.TTL <= 0 {
		req.TTL = p.cfg.TokenTTL
	}
	t, err := p.IssueToken(req.Container, req.TTL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"token": t})
}

func (p *Proxy) handleAdminRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !p.requireAdmin(w, r) {
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

func (p *Proxy) handleAdminMode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !p.requireAdmin(w, r) {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{
		"oauth_mode":           p.IsOAuthMode(),
		"anthropic_available":  p.IsAnthropicMode(),
		"anthropic_oauth_mode": p.IsAnthropicOAuthMode(),
	})
}

// Stop shuts down the proxy and revokes all tokens.
func (p *Proxy) Stop() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if p.server != nil {
		_ = p.server.Shutdown(ctx)
	}
	if p.adminServer != nil {
		_ = p.adminServer.Shutdown(ctx)
	}
	p.mu.Lock()
	p.tokens = make(map[string]*tokenRecord)
	p.mu.Unlock()
}

// Port returns the TCP port the proxy is listening on.
// Only valid after Start() has been called.
func (p *Proxy) Port() string {
	_, port, _ := net.SplitHostPort(p.addr)
	return port
}

// Endpoint returns the proxy URL for host-side access (e.g. health checks, tests).
// Always uses 127.0.0.1 regardless of bind address.
func (p *Proxy) Endpoint() string {
	return "http://127.0.0.1:" + p.Port()
}

// ContainerEndpoint returns the proxy URL that worker containers should use.
// Containers reach the proxy via host.docker.internal which Docker resolves
// to the host's bridge gateway IP at container creation time.
// Requires Docker Engine >= 20.10 and --add-host=host.docker.internal:host-gateway
// to be set on the worker container (manager.go handles this automatically).
func (p *Proxy) ContainerEndpoint() string {
	return "http://host.docker.internal:" + p.Port()
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
		// OAuth mode: return a placeholder access_token (the container's CODEX_TOKEN).
		// The real access_token never leaves the proxy; it is injected by
		// reverseProxy/handleWebSocketProxy on every outbound API request.
		// id_token is passed through so Codex CLI can extract account/plan claims
		// for UI display and ChatGPT-Account-Id header construction.
		// refresh_token is intentionally withheld.
		p.mu.RLock()
		creds := *p.oauthCreds
		p.mu.RUnlock()
		_ = json.NewEncoder(w).Encode(map[string]string{
			"oauth_access_token": found.Token, // placeholder; real token injected by proxy
			"oauth_id_token":     creds.IDToken,
			"oauth_account_id":   creds.AccountID,
			"oauth_last_refresh": creds.LastRefresh,
			"container_name":     found.ContainerName,
			// oauth_refresh_token intentionally omitted
		})
	} else {
		// API key mode: return a placeholder api_key (the container's CODEX_TOKEN).
		// The real API key is injected by reverseProxy on every outbound request.
		_ = json.NewEncoder(w).Encode(map[string]string{
			"api_key":        found.Token, // placeholder; real key injected by proxy
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

// handleAnthropicProxy proxies /anthropic/* to https://api.anthropic.com/*.
// Containers set ANTHROPIC_BASE_URL=http://<proxy>/anthropic so Claude Code
// routes all Messages API traffic through the proxy. The proxy injects the
// host's real Anthropic credential (API key or OAuth bearer) on every request,
// replacing whatever placeholder the container sent.
func (p *Proxy) handleAnthropicProxy(w http.ResponseWriter, r *http.Request) {
	p.reverseProxy(w, r, "/anthropic", p.anthropicUpstreamURL, p.injectAnthropicCredentials)
}

// handleChatGPTProxy proxies /chatgpt/* to https://chatgpt.com/backend-api/*.
// Containers in ChatGPT auth mode set chatgpt_base_url=http://<proxy>/chatgpt/
// in their Codex CLI config so backend-api calls (rate limits, account info, etc.)
// flow through the proxy.
func (p *Proxy) handleChatGPTProxy(w http.ResponseWriter, r *http.Request) {
	p.reverseProxy(w, r, "/chatgpt", p.chatgptURL, p.injectCredentials)
}

// isWebSocketRequest returns true when the incoming request is a WebSocket upgrade.
func isWebSocketRequest(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket") &&
		strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade")
}

// handleWebSocketProxy tunnels a WebSocket connection to targetURLStr.
// It hijacks the incoming HTTP/1.1 connection, opens a raw TCP (or TLS) connection
// to the upstream, performs the HTTP upgrade handshake, then copies data
// bidirectionally between client and upstream.
func (p *Proxy) handleWebSocketProxy(w http.ResponseWriter, r *http.Request, targetURLStr string) {
	targetURL, err := url.Parse(targetURLStr)
	if err != nil {
		http.Error(w, "invalid target URL", http.StatusInternalServerError)
		return
	}

	host := targetURL.Host
	useTLS := targetURL.Scheme == "https" || targetURL.Scheme == "wss"
	if _, _, err := net.SplitHostPort(host); err != nil {
		if useTLS {
			host += ":443"
		} else {
			host += ":80"
		}
	}

	var upstreamConn net.Conn
	if useTLS {
		d := &tls.Dialer{Config: &tls.Config{ServerName: targetURL.Hostname()}}
		upstreamConn, err = d.DialContext(r.Context(), "tcp", host)
	} else {
		d := &net.Dialer{}
		upstreamConn, err = d.DialContext(r.Context(), "tcp", host)
	}
	if err != nil {
		http.Error(w, "connecting to upstream WebSocket: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer upstreamConn.Close() //nolint:errcheck

	// Send the HTTP upgrade request to the upstream, forwarding all client headers.
	// Authorization and ChatGPT-Account-Id are skipped here and replaced with
	// real host credentials below so placeholder tokens never reach the upstream.
	reqURI := targetURL.RequestURI()
	if _, err := fmt.Fprintf(upstreamConn, "%s %s HTTP/1.1\r\nHost: %s\r\n", r.Method, reqURI, targetURL.Host); err != nil {
		http.Error(w, "writing upgrade request: "+err.Error(), http.StatusBadGateway)
		return
	}
	for k, vv := range r.Header {
		if strings.EqualFold(k, "authorization") || strings.EqualFold(k, "chatgpt-account-id") {
			continue
		}
		for _, v := range vv {
			if _, err := fmt.Fprintf(upstreamConn, "%s: %s\r\n", k, v); err != nil {
				http.Error(w, "writing headers: "+err.Error(), http.StatusBadGateway)
				return
			}
		}
	}
	// Inject real host credentials.
	if p.oauthCreds != nil {
		p.mu.RLock()
		accessToken := p.oauthCreds.AccessToken
		accountID := p.oauthCreds.AccountID
		p.mu.RUnlock()
		if _, err := fmt.Fprintf(upstreamConn, "Authorization: Bearer %s\r\n", accessToken); err != nil {
			http.Error(w, "writing auth header: "+err.Error(), http.StatusBadGateway)
			return
		}
		if accountID != "" {
			if _, err := fmt.Fprintf(upstreamConn, "ChatGPT-Account-Id: %s\r\n", accountID); err != nil {
				http.Error(w, "writing account-id header: "+err.Error(), http.StatusBadGateway)
				return
			}
		}
	} else if p.apiKey != "" {
		if _, err := fmt.Fprintf(upstreamConn, "Authorization: Bearer %s\r\n", p.apiKey); err != nil {
			http.Error(w, "writing auth header: "+err.Error(), http.StatusBadGateway)
			return
		}
	}
	if _, err := fmt.Fprint(upstreamConn, "\r\n"); err != nil {
		http.Error(w, "writing header terminator: "+err.Error(), http.StatusBadGateway)
		return
	}

	// Read the upstream's 101 Switching Protocols response.
	upstreamBufReader := bufio.NewReader(upstreamConn)
	upstreamResp, err := http.ReadResponse(upstreamBufReader, r)
	if err != nil {
		http.Error(w, "reading upstream upgrade response: "+err.Error(), http.StatusBadGateway)
		return
	}
	if upstreamResp.StatusCode != http.StatusSwitchingProtocols {
		// Upstream rejected the upgrade; relay the error response to the client.
		for k, vv := range upstreamResp.Header {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(upstreamResp.StatusCode)
		_, _ = io.Copy(w, upstreamResp.Body)
		_ = upstreamResp.Body.Close()
		return
	}
	_ = upstreamResp.Body.Close()

	// Hijack the client connection to take over the raw TCP stream.
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "WebSocket proxying requires HTTP/1.1", http.StatusInternalServerError)
		return
	}
	clientConn, clientBufRW, err := hj.Hijack()
	if err != nil {
		log.Printf("WebSocket hijack error: %v", err)
		return
	}
	defer clientConn.Close() //nolint:errcheck

	// Forward the 101 response to the client.
	if _, err := fmt.Fprint(clientBufRW, "HTTP/1.1 101 Switching Protocols\r\n"); err != nil {
		log.Printf("WebSocket: writing 101 to client: %v", err)
		return
	}
	for k, vv := range upstreamResp.Header {
		for _, v := range vv {
			if _, err := fmt.Fprintf(clientBufRW, "%s: %s\r\n", k, v); err != nil {
				log.Printf("WebSocket: writing 101 headers to client: %v", err)
				return
			}
		}
	}
	if _, err := fmt.Fprint(clientBufRW, "\r\n"); err != nil {
		log.Printf("WebSocket: writing 101 terminator to client: %v", err)
		return
	}
	if err := clientBufRW.Flush(); err != nil {
		log.Printf("WebSocket: flushing 101 to client: %v", err)
		return
	}

	if p.cfg.Verbose {
		fmt.Printf("WebSocket tunnel established: %s <-> %s\n", r.URL.Path, targetURLStr)
	}

	// Bidirectional tunnel.
	// upstreamBufReader drains any bytes already buffered after the 101 headers
	// before falling through to raw reads from upstreamConn.
	errc := make(chan error, 2)
	go func() {
		_, err := io.Copy(clientConn, upstreamBufReader)
		errc <- err
	}()
	go func() {
		_, err := io.Copy(upstreamConn, clientConn)
		errc <- err
	}()
	<-errc
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

// injectAnthropicCredentials overwrites the Anthropic auth headers in h with the
// host's real credentials, replacing any placeholder the container sent.
//   - API-key mode: sets x-api-key, removes Authorization.
//   - OAuth mode:  sets Authorization: Bearer <access_token>, removes x-api-key,
//     and adds the required anthropic-beta OAuth header.
//
// In OAuth mode the access token is refreshed first if it has expired.
// Safe to call concurrently.
func (p *Proxy) injectAnthropicCredentials(h http.Header) {
	// Anthropic requires anthropic-version; supply a default if the client omits it.
	if h.Get("anthropic-version") == "" {
		h.Set("anthropic-version", defaultAnthropicVersion)
	}

	if p.anthropicOAuth != nil {
		p.refreshAnthropicOAuthIfNeeded(context.Background())
		p.mu.RLock()
		accessToken := p.anthropicOAuth.AccessToken
		p.mu.RUnlock()
		h.Del("x-api-key")
		h.Set("Authorization", "Bearer "+accessToken)
		mergeBetaHeader(h, anthropicOAuthBetaHeader)
		return
	}
	if p.anthropicAPIKey != "" {
		h.Del("Authorization")
		h.Set("x-api-key", p.anthropicAPIKey)
	}
}

// mergeBetaHeader ensures value is present in the anthropic-beta header without
// dropping any beta flags the client already requested.
func mergeBetaHeader(h http.Header, value string) {
	existing := h.Get("anthropic-beta")
	if existing == "" {
		h.Set("anthropic-beta", value)
		return
	}
	for _, part := range strings.Split(existing, ",") {
		if strings.EqualFold(strings.TrimSpace(part), value) {
			return
		}
	}
	h.Set("anthropic-beta", existing+","+value)
}

// refreshAnthropicOAuthIfNeeded refreshes the host's Anthropic OAuth access token
// when it is missing an expiry or within 60s of expiring. The refresh token stays
// on the host; only the resulting access token is ever injected into requests.
func (p *Proxy) refreshAnthropicOAuthIfNeeded(ctx context.Context) {
	p.mu.RLock()
	if p.anthropicOAuth == nil {
		p.mu.RUnlock()
		return
	}
	expiresAt := p.anthropicOAuth.ExpiresAt
	refreshToken := p.anthropicOAuth.RefreshToken
	p.mu.RUnlock()

	// expiresAt == 0 means unknown; in that case do not attempt a refresh
	// (the current token is assumed valid).
	if expiresAt == 0 || refreshToken == "" {
		return
	}
	if time.Now().UnixMilli() < expiresAt-60_000 {
		return
	}

	body, _ := json.Marshal(map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": refreshToken,
		"client_id":     p.anthropicOAuthClientID,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.anthropicOAuthTokenURL, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		if p.cfg.Verbose {
			fmt.Printf("Anthropic OAuth refresh failed: %v\n", err)
		}
		return
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode != http.StatusOK {
		if p.cfg.Verbose {
			fmt.Printf("Anthropic OAuth refresh returned status %d\n", resp.StatusCode)
		}
		return
	}

	var tr struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(respBody, &tr); err != nil || tr.AccessToken == "" {
		return
	}

	p.mu.Lock()
	if p.anthropicOAuth != nil {
		p.anthropicOAuth.AccessToken = tr.AccessToken
		if tr.RefreshToken != "" {
			p.anthropicOAuth.RefreshToken = tr.RefreshToken
		}
		if tr.ExpiresIn > 0 {
			p.anthropicOAuth.ExpiresAt = time.Now().UnixMilli() + tr.ExpiresIn*1000
		}
	}
	p.mu.Unlock()
	if p.cfg.Verbose {
		fmt.Println("Anthropic OAuth access token refreshed")
	}
}

// reverseProxy strips stripPrefix from r.URL.Path, appends it to targetBase,
// and forwards the request upstream, copying status, headers, and body back.
// inject overwrites auth headers with the host's real credentials before the
// request leaves the proxy.
func (p *Proxy) reverseProxy(w http.ResponseWriter, r *http.Request, stripPrefix, targetBase string, inject func(http.Header)) {
	path := strings.TrimPrefix(r.URL.Path, stripPrefix)
	target := targetBase + path
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}

	// WebSocket upgrade requests require raw TCP tunneling, not HTTP proxying.
	if isWebSocketRequest(r) {
		p.handleWebSocketProxy(w, r, target)
		return
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
	// Inject real host credentials, overriding any placeholder the container sent.
	inject(req.Header)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		http.Error(w, "upstream error: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	copyAndFlush(w, resp.Body)

	if p.cfg.Verbose {
		fmt.Printf("Proxied %s %s -> %s (%d)\n", r.Method, r.URL.Path, target, resp.StatusCode)
	}
}

// copyAndFlush streams src to w, flushing after every chunk when w supports
// http.Flusher. This preserves real-time delivery of Server-Sent Events
// (text/event-stream), which both the Anthropic Messages API and the OpenAI
// Responses API use for streaming responses. A plain io.Copy would let the
// net/http response buffer hold events until it fills or the handler returns,
// stalling interactive agents.
func copyAndFlush(w http.ResponseWriter, src io.Reader) {
	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 32*1024)
	for {
		n, rerr := src.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if rerr != nil {
			return
		}
	}
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "cdx-" + hex.EncodeToString(b), nil
}
