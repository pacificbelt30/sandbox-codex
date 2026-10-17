package sandbox

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/pacificbelt30/codex-dock/internal/authproxy"
)

func (m *Manager) workerProxy(v container.InspectResponse) (authproxy.Service, error) {
	if m.proxy != nil {
		return m.proxy, nil
	}
	if m.proxyConnector == nil {
		return nil, fmt.Errorf("auth proxy connection is required to restart this worker")
	}
	return m.proxyConnector(v.Config.Labels[labelPrefix+"proxy-admin-url"], v.Config.Labels[labelPrefix+"proxy-container-url"])
}

func workerPlaceholder(v container.InspectResponse) string {
	if v.Config == nil {
		return ""
	}
	for _, entry := range v.Config.Env {
		key, value, ok := strings.Cut(entry, "=")
		if ok && (key == "CODEX_TOKEN" || key == "ANTHROPIC_API_KEY") && strings.HasPrefix(value, "cdx-") {
			return value
		}
	}
	return ""
}

func (m *Manager) restoreWorkerToken(v container.InspectResponse) error {
	token := workerPlaceholder(v)
	if token == "" {
		return nil
	}
	p, err := m.workerProxy(v)
	if err != nil {
		return err
	}
	r, ok := p.(authproxy.TokenRestorer)
	if !ok {
		return fmt.Errorf("auth proxy does not support worker restart; update the proxy")
	}
	ttl, _ := strconv.Atoi(v.Config.Labels[labelPrefix+"token-ttl"])
	if ttl <= 0 {
		ttl = 3600
	}
	return r.RestoreToken(strings.TrimPrefix(v.Name, "/"), token, ttl)
}

func (m *Manager) revokeWorkerToken(v container.InspectResponse) {
	if err := m.revokeWorkerTokenChecked(v); err != nil {
		fmt.Fprintln(os.Stderr, "warning: worker token could not be revoked:", err)
	}
}

func (m *Manager) revokeWorkerTokenChecked(v container.InspectResponse) error {
	if workerPlaceholder(v) == "" {
		return nil
	}
	p, err := m.workerProxy(v)
	if err != nil {
		return err
	}
	name := strings.TrimPrefix(v.Name, "/")
	if revoker, ok := p.(authproxy.TokenRevoker); ok {
		return revoker.RevokeTokenChecked(name)
	}
	p.RevokeToken(name)
	return nil
}
