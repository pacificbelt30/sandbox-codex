package authproxy

import (
	"encoding/json"
	"net/http"
)

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
		"worker_token_auth":    true,
		"oauth_mode":           p.IsOAuthMode(),
		"anthropic_available":  p.IsAnthropicMode(),
		"anthropic_oauth_mode": p.IsAnthropicOAuthMode(),
	})
}
