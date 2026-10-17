package authproxy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// tokenRecord holds a single issued token and its metadata.
type tokenRecord struct {
	Token         string
	ContainerName string
	ExpiresAt     time.Time
	IssuedAt      time.Time
	context       context.Context
	cancel        context.CancelFunc
}

func newTokenRecord(name, token string, ttl int) *tokenRecord {
	now := time.Now()
	expires := now.Add(time.Duration(ttl) * time.Second)
	ctx, cancel := context.WithDeadline(context.Background(), expires)
	return &tokenRecord{Token: token, ContainerName: name, ExpiresAt: expires, IssuedAt: now, context: ctx, cancel: cancel}
}

// IssueToken creates and stores a short-lived token for a container.
func (p *Proxy) IssueToken(containerName string, ttlSec int) (string, error) {
	token, err := generateToken()
	if err != nil {
		return "", err
	}

	rec := newTokenRecord(containerName, token, ttlSec)

	p.mu.Lock()
	if old := p.tokens[containerName]; old != nil && old.cancel != nil {
		old.cancel()
	}
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
	if rec := p.tokens[containerName]; rec != nil && rec.cancel != nil {
		rec.cancel()
	}
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
				if rec.cancel != nil {
					rec.cancel()
				}
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
