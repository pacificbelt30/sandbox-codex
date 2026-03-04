package authproxy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
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
}

// NewProxy creates a new Auth Proxy.
// In OAuth mode (detected via IsOAuthAuth), credentials are loaded from
// ~/.codex/auth.json. In API key mode, the API key is loaded as usual.
func NewProxy(cfg Config) (*Proxy, error) {
	p := &Proxy{
		cfg:    cfg,
		tokens: make(map[string]*tokenRecord),
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
		// OAuth mode: return only the access_token — refresh_token stays on the host.
		p.mu.RLock()
		accessToken := p.oauthCreds.AccessToken
		p.mu.RUnlock()
		_ = json.NewEncoder(w).Encode(map[string]string{
			"oauth_access_token": accessToken,
			"container_name":     found.ContainerName,
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

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "cdx-" + hex.EncodeToString(b), nil
}
