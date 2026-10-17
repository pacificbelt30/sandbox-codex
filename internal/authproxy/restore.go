package authproxy

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func (p *Proxy) RestoreToken(name, token string, ttl int) error {
	if name == "" || len(name) > 255 || strings.ContainsAny(name, "\r\n\x00") || !strings.HasPrefix(token, "cdx-") || len(token) != 68 {
		return fmt.Errorf("invalid worker placeholder")
	}
	if _, err := hex.DecodeString(strings.TrimPrefix(token, "cdx-")); err != nil {
		return fmt.Errorf("invalid worker placeholder")
	}
	if ttl <= 0 {
		ttl = p.cfg.TokenTTL
	}
	if ttl <= 0 || ttl > 365*24*3600 {
		return fmt.Errorf("invalid token TTL")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for owner, rec := range p.tokens {
		if owner != name && rec.Token == token {
			return fmt.Errorf("placeholder already belongs to another worker")
		}
	}
	now := time.Now()
	p.tokens[name] = &tokenRecord{Token: token, ContainerName: name, IssuedAt: now, ExpiresAt: now.Add(time.Duration(ttl) * time.Second)}
	return nil
}

func (p *Proxy) handleAdminRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	if !p.requireAdmin(w, r) {
		return
	}
	var req struct {
		Container string `json:"container"`
		Token     string `json:"token"`
		TTL       int    `json:"ttl"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&req); err != nil {
		http.Error(w, "invalid json", 400)
		return
	}
	if err := p.RestoreToken(req.Container, req.Token, req.TTL); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (r *RemoteProxy) RestoreToken(name, token string, ttl int) error {
	body, err := json.Marshal(map[string]any{"container": name, "token": token, "ttl": ttl})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, r.adminURL+"/admin/restore", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if r.adminSecret != "" {
		req.Header.Set(r.adminSecretHeader, r.adminSecret)
	}
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("restoring worker token failed (status %d); rebuild/restart the auth proxy with this version", resp.StatusCode)
	}
	return nil
}
