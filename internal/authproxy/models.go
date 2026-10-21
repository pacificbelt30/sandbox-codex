package authproxy

import (
	"bytes"
	"encoding/json"
	"github.com/pacificbelt30/codex-dock/pkg/cxd/identity"
	"io"
	"mime"
	"net/http"
	"slices"
	"strings"
)

const modelBodyLimit = 8 << 20

// ValidateAllowedModels accepts exact upstream IDs only. Empty preserves the
// unrestricted legacy proxy. No alias expansion or wildcard matching occurs.
func ValidateAllowedModels(models []string) error {
	return (&identity.ModelPolicy{AllowedModels: models}).Validate()
}

// authorizeModel runs after token authentication and before credential injection
// or upstream I/O. Unsupported transports and endpoints fail closed when enabled.
func (p *Proxy) authorizeModel(w http.ResponseWriter, r *http.Request) bool {
	policy, _ := r.Context().Value(modelPolicyKey{}).(*identity.ModelPolicy)
	if !p.modelPolicyActive(r) {
		return true
	}
	deny := func(code string, status int) bool {
		w.Header().Set("X-Cxd-Policy-Reason", code)
		http.Error(w, code, status)
		return false
	}
	if isWebSocketRequest(r) {
		return deny("model_policy_websocket_unsupported", http.StatusForbidden)
	}
	if r.Method != http.MethodPost || r.URL.RawQuery != "" || r.URL.EscapedPath() != r.URL.Path || r.Header.Get("Content-Encoding") != "" {
		return deny("model_policy_route_denied", http.StatusForbidden)
	}
	switch r.URL.Path {
	case "/v1/responses", "/v1/responses/compact", "/v1/chat/completions", "/v1/completions", "/v1/embeddings", "/v1/moderations", "/chatgpt/codex/responses", "/chatgpt/codex/responses/compact", "/anthropic/v1/messages", "/anthropic/v1/messages/count_tokens":
	default:
		return deny("model_policy_route_denied", http.StatusForbidden)
	}
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" || r.Body == nil {
		return deny("model_policy_invalid_body", http.StatusBadRequest)
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, modelBodyLimit+1))
	_ = r.Body.Close()
	if err != nil {
		return deny("model_policy_invalid_body", http.StatusBadRequest)
	}
	if len(body) > modelBodyLimit {
		return deny("model_policy_body_limit", http.StatusRequestEntityTooLarge)
	}
	// Decode each top-level member separately to reject duplicate/case-variant
	// keys. Otherwise an upstream parser could select a different model value.
	d := json.NewDecoder(bytes.NewReader(body))
	first, err := d.Token()
	if err != nil || first != json.Delim('{') {
		return deny("model_policy_invalid_body", http.StatusBadRequest)
	}
	seen := make(map[string]bool)
	var model string
	for d.More() {
		key, err := d.Token()
		if err != nil {
			return deny("model_policy_invalid_body", http.StatusBadRequest)
		}
		name, ok := key.(string)
		if !ok || seen[name] || strings.EqualFold(name, "model") && name != "model" {
			return deny("model_policy_invalid_body", http.StatusBadRequest)
		}
		seen[name] = true
		var raw json.RawMessage
		if err := d.Decode(&raw); err != nil {
			return deny("model_policy_invalid_body", http.StatusBadRequest)
		}
		if name == "model" {
			if err := json.Unmarshal(raw, &model); err != nil {
				return deny("model_policy_invalid_body", http.StatusBadRequest)
			}
		}
	}
	if _, err := d.Token(); err != nil {
		return deny("model_policy_invalid_body", http.StatusBadRequest)
	}
	var extra json.RawMessage
	if err := d.Decode(&extra); err != io.EOF {
		return deny("model_policy_invalid_body", http.StatusBadRequest)
	}
	if model == "" || len(p.cfg.AllowedModels) != 0 && !slices.Contains(p.cfg.AllowedModels, model) || policy != nil && !slices.Contains(policy.AllowedModels, model) {
		return deny("model_not_allowed", http.StatusForbidden)
	}

	r.Body = io.NopCloser(bytes.NewReader(body))
	r.ContentLength = int64(len(body))
	return true
}

type modelPolicyKey struct{}

func (p *Proxy) modelPolicyActive(r *http.Request) bool {
	policy, _ := r.Context().Value(modelPolicyKey{}).(*identity.ModelPolicy)
	return len(p.cfg.AllowedModels) != 0 || policy != nil
}
