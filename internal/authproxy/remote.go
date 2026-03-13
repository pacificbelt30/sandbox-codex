package authproxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// RemoteProxy talks to an externally running auth proxy service.
type RemoteProxy struct {
	adminURL          string
	containerURL      string
	adminSecret       string
	oauthMode         bool
	httpClient        *http.Client
	adminSecretHeader string
}

func NewRemoteProxy(adminURL, containerURL, adminSecret string) (*RemoteProxy, error) {
	r := &RemoteProxy{
		adminURL:          adminURL,
		containerURL:      containerURL,
		adminSecret:       adminSecret,
		httpClient:        http.DefaultClient,
		adminSecretHeader: "X-Proxy-Admin-Secret",
	}
	mode, err := r.fetchOAuthMode()
	if err != nil {
		return nil, err
	}
	r.oauthMode = mode
	return r, nil
}

func (r *RemoteProxy) ContainerEndpoint() string { return r.containerURL }
func (r *RemoteProxy) IsOAuthMode() bool         { return r.oauthMode }

func (r *RemoteProxy) IssueToken(containerName string, ttlSec int) (string, error) {
	body, _ := json.Marshal(map[string]interface{}{"container": containerName, "ttl": ttlSec})
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, r.adminURL+"/admin/issue", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if r.adminSecret != "" {
		req.Header.Set(r.adminSecretHeader, r.adminSecret)
	}
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("issue token failed: status %d", resp.StatusCode)
	}
	var out map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out["token"], nil
}

func (r *RemoteProxy) RevokeToken(containerName string) {
	u, _ := url.Parse(r.adminURL + "/admin/revoke")
	q := u.Query()
	q.Set("container", containerName)
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, u.String(), nil)
	if err != nil {
		return
	}
	if r.adminSecret != "" {
		req.Header.Set(r.adminSecretHeader, r.adminSecret)
	}
	resp, err := r.httpClient.Do(req)
	if err == nil && resp != nil {
		_ = resp.Body.Close()
	}
}

func (r *RemoteProxy) fetchOAuthMode() (bool, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, r.adminURL+"/admin/mode", nil)
	if err != nil {
		return false, err
	}
	if r.adminSecret != "" {
		req.Header.Set(r.adminSecretHeader, r.adminSecret)
	}
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("connecting remote auth proxy (%s): %w%s", r.adminURL, err, connectionHint(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("checking remote auth proxy mode failed: status %d", resp.StatusCode)
	}
	var out map[string]bool
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return false, err
	}
	return out["oauth_mode"], nil
}

func connectionHint(err error) string {
	var netErr *net.OpError
	if !errors.As(err, &netErr) {
		return ""
	}
	if !strings.Contains(strings.ToLower(netErr.Err.Error()), "connection refused") {
		return ""
	}
	return "\nHint: auth proxy is not running. Start it with `codex-dock proxy serve --listen 0.0.0.0:18080` or run the Docker quick-start command in doc/getting-started.md."
}
