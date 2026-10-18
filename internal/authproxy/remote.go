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
	"time"
)

// RemoteProxy talks to an externally running auth proxy service.
type RemoteProxy struct {
	adminURL           string
	containerURL       string
	adminSecret        string
	oauthMode          bool
	anthropicMode      bool
	anthropicOAuthMode bool
	httpClient         *http.Client
	adminSecretHeader  string
}

func NewRemoteProxy(adminURL, containerURL, adminSecret string) (*RemoteProxy, error) {
	r := &RemoteProxy{
		adminURL:          adminURL,
		containerURL:      containerURL,
		adminSecret:       adminSecret,
		httpClient:        &http.Client{Timeout: 10 * time.Second},
		adminSecretHeader: "X-Proxy-Admin-Secret",
	}
	mode, err := r.fetchMode()
	if err != nil {
		return nil, err
	}
	if !mode["worker_token_auth"] {
		return nil, fmt.Errorf("auth proxy does not enforce worker API tokens; rebuild the proxy image and recreate the proxy before running workers")
	}
	r.oauthMode = mode["oauth_mode"]
	r.anthropicMode = mode["anthropic_available"]
	r.anthropicOAuthMode = mode["anthropic_oauth_mode"]
	return r, nil
}

func (r *RemoteProxy) ContainerEndpoint() string  { return r.containerURL }
func (r *RemoteProxy) IsOAuthMode() bool          { return r.oauthMode }
func (r *RemoteProxy) IsAnthropicMode() bool      { return r.anthropicMode }
func (r *RemoteProxy) IsAnthropicOAuthMode() bool { return r.anthropicOAuthMode }

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
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("issue token failed: status %d", resp.StatusCode)
	}
	var out map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out["token"], nil
}

func (r *RemoteProxy) RevokeToken(containerName string) { _ = r.RevokeTokenChecked(containerName) }

func (r *RemoteProxy) RevokeTokenChecked(containerName string) error {
	u, _ := url.Parse(r.adminURL + "/admin/revoke")
	q := u.Query()
	q.Set("container", containerName)
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, u.String(), nil)
	if err != nil {
		return err
	}
	if r.adminSecret != "" {
		req.Header.Set(r.adminSecretHeader, r.adminSecret)
	}
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("proxy token revocation failed: HTTP %d", resp.StatusCode)
	}
	return nil
}

func (r *RemoteProxy) fetchMode() (map[string]bool, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, r.adminURL+"/admin/mode", nil)
	if err != nil {
		return nil, err
	}
	if r.adminSecret != "" {
		req.Header.Set(r.adminSecretHeader, r.adminSecret)
	}
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("connecting remote auth proxy (%s): %w%s", r.adminURL, err, connectionHint(err))
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("checking remote auth proxy mode failed: status %d", resp.StatusCode)
	}
	var out map[string]bool
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func connectionHint(err error) string {
	var netErr *net.OpError
	if !errors.As(err, &netErr) {
		return ""
	}
	if !strings.Contains(strings.ToLower(netErr.Err.Error()), "connection refused") {
		return ""
	}
	return "\nHint: auth proxy is not running. Run `codex-dock init` or `codex-dock proxy run` on the prepared containerd host."
}
