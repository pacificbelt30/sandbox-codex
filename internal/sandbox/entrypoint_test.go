package sandbox

import (
	"os"
	"strings"
	"testing"
)

func TestEntrypointWritesCodexConfigToCodexHome(t *testing.T) {
	entrypoint, err := os.ReadFile("../../docker/sandbox/entrypoint.sh")
	if err != nil {
		t.Fatalf("read entrypoint: %v", err)
	}

	script := string(entrypoint)
	if !strings.Contains(script, `"${CODEX_HOME}/.codex/config.toml"`) {
		t.Error("entrypoint must write config.toml under ~/.codex")
	}
	if strings.Contains(script, `"${CODEX_HOME}/.config/codex/config.toml"`) {
		t.Error("entrypoint writes config.toml to legacy ~/.config/codex path")
	}
}

// TestEntrypointSetsOpenAIBaseURLInConfig guards against regressing to the
// (broken) assumption that the OPENAI_BASE_URL environment variable redirects
// Codex CLI. Codex resolves the Responses API endpoint solely from the
// openai_base_url config key (openai/codex#16719), so the entrypoint must write
// it to config.toml. chatgpt_base_url alone is insufficient — it only affects the
// ChatGPT login/backend-api flow, not model requests.
func TestEntrypointSetsOpenAIBaseURLInConfig(t *testing.T) {
	entrypoint, err := os.ReadFile("../../docker/sandbox/entrypoint.sh")
	if err != nil {
		t.Fatalf("read entrypoint: %v", err)
	}

	script := string(entrypoint)
	if !strings.Contains(script, `openai_base_url = "${CODEX_AUTH_PROXY_URL}/v1"`) {
		t.Error("entrypoint must set openai_base_url in config.toml to route the Responses API through the proxy")
	}
	// openai_base_url is required in both OAuth and API-key modes; the string must
	// appear at least twice (once per branch).
	if n := strings.Count(script, `openai_base_url = "${CODEX_AUTH_PROXY_URL}/v1"`); n < 2 {
		t.Errorf("openai_base_url must be written in both OAuth and API-key modes, found %d occurrence(s)", n)
	}
}

func TestEntrypointDisablesAppsInConfig(t *testing.T) {
	entrypoint, err := os.ReadFile("../../docker/sandbox/entrypoint.sh")
	if err != nil {
		t.Fatalf("read entrypoint: %v", err)
	}

	script := string(entrypoint)
	if n := strings.Count(script, "[features]\napps = false"); n != 2 {
		t.Errorf("apps must be disabled in both OAuth and API-key modes, found %d occurrence(s)", n)
	}
}
