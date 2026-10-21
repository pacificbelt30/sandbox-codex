package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pacificbelt30/codex-dock/pkg/cxd/identity"
)

// WorkerConnection contains only the worker's saved short-lived placeholder.
// It never loads host credentials or exports the proxy's admin secret.
type WorkerConnection struct {
	Provider, BaseURL, Placeholder string
	ModelPolicyActive              bool
}

func (m *Manager) WorkerConnections(ctx context.Context, reference string) ([]WorkerConnection, error) {
	v, err := m.cli.ContainerInspect(ctx, reference)
	if err != nil {
		return nil, err
	}
	if v.Config == nil || v.Config.Labels[labelManaged] != "true" {
		return nil, fmt.Errorf("worker is not managed")
	}
	p, err := m.workerProxyContext(ctx, v)
	if err != nil {
		return nil, err
	}
	restricted := v.Config.Labels[labelPrefix+"token-model-policy"] != ""
	if source, ok := p.(interface{ ModelPolicyActive() bool }); ok {
		restricted = restricted || source.ModelPolicyActive()
	}
	var id identity.Identity
	if raw := v.Config.Labels[labelPrefix+"token-identity"]; raw != "" {
		if err := json.Unmarshal([]byte(raw), &id); err != nil {
			return nil, fmt.Errorf("invalid saved identity: %w", err)
		}
		if err := id.Validate(); err != nil {
			return nil, err
		}
	}
	env := make(map[string]string)
	for _, entry := range v.Config.Env {
		if key, value, ok := strings.Cut(entry, "="); ok {
			env[key] = value
		}
	}
	var result []WorkerConnection
	if endpoint, token := env["CODEX_AUTH_PROXY_URL"], env["CODEX_TOKEN"]; endpoint != "" && strings.HasPrefix(token, "cdx-") && (id.Provider == "" || id.Provider == "openai") {
		result = append(result, WorkerConnection{"openai", strings.TrimRight(endpoint, "/") + "/v1", token, restricted})
	}
	if endpoint, token := env["ANTHROPIC_BASE_URL"], env["ANTHROPIC_API_KEY"]; endpoint != "" && strings.HasPrefix(token, "cdx-") && (id.Provider == "" || id.Provider == "anthropic") {
		result = append(result, WorkerConnection{"anthropic", endpoint, token, restricted})
	}
	return result, nil
}
