package authproxy

import (
	"encoding/json"
	"net/http"

	"github.com/pacificbelt30/codex-dock/pkg/cxd/identity"
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
		Container string            `json:"container"`
		TTL       int               `json:"ttl"`
		Identity  identity.Identity `json:"identity"`
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
	t, err := p.IssueTokenWithIdentity(req.Container, req.TTL, req.Identity)
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

func (p *Proxy) handleAdminRevokeRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !p.requireAdmin(w, r) {
		return
	}
	var in identity.Identity
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&in); err != nil || in.Validate() != nil || p.cfg.CaseID != "" && in.CaseID != p.cfg.CaseID {
		http.Error(w, "invalid run identity", http.StatusBadRequest)
		return
	}
	p.mu.Lock()
	if p.revokedRuns == nil {
		p.revokedRuns = make(map[identity.Identity]bool)
	}
	p.revokedRuns[in] = true
	for name, rec := range p.tokens {
		if rec.Identity == in {
			rec.cancel()
			delete(p.tokens, name)
		}
	}
	p.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (p *Proxy) handleAdminRevokeCase(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !p.requireAdmin(w, r) {
		return
	}
	if p.cfg.CaseID == "" {
		http.Error(w, "not a case proxy", http.StatusBadRequest)
		return
	}
	p.revokeCaseTokens()
	w.WriteHeader(http.StatusNoContent)
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
		"token_identity":       true,
		"case_proxy":           p.cfg.CredentialStore != nil,
	})
}
