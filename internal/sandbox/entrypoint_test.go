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

// TestEntrypointUsesDockShellForInteractiveShell guards the no-agent shell
// case: it must launch the shell selected via DOCK_SHELL (defaulting to bash,
// and falling back to bash via resolve_shell if DOCK_SHELL names a binary
// that isn't installed in the image) rather than a hardcoded "bash -i".
func TestEntrypointUsesDockShellForInteractiveShell(t *testing.T) {
	entrypoint, err := os.ReadFile("../../docker/sandbox/entrypoint.sh")
	if err != nil {
		t.Fatalf("read entrypoint: %v", err)
	}

	script := string(entrypoint)
	if !strings.Contains(script, `exec "${SHELL_BIN}" -i`) {
		t.Error(`entrypoint's no-agent shell case must exec "${SHELL_BIN}" -i (SHELL_BIN from resolve_shell, which reads DOCK_SHELL)`)
	}
	if !strings.Contains(script, `SHELL_BIN="$(resolve_shell)"`) {
		t.Error("entrypoint's no-agent branch must set SHELL_BIN via resolve_shell")
	}
	if !strings.Contains(script, `local requested="${DOCK_SHELL:-bash}"`) {
		t.Error("resolve_shell must read the requested shell from DOCK_SHELL, defaulting to bash")
	}
	if !strings.Contains(script, `command -v "$requested"`) {
		t.Error("resolve_shell must verify the requested shell exists before using it, falling back to bash otherwise")
	}
}

func TestEntrypointPreventsZshNewUserWizard(t *testing.T) {
	entrypoint, err := os.ReadFile("../../docker/sandbox/entrypoint.sh")
	if err != nil {
		t.Fatalf("read entrypoint: %v", err)
	}

	script := string(entrypoint)
	if !strings.Contains(script, `ensure_zsh_startup_file "${SHELL_BIN}"`) {
		t.Error("entrypoint must ensure a startup file exists before launching interactive zsh")
	}
	if !strings.Contains(script, `[[ ! -e "${startup_dir}/.zshrc" ]]`) {
		t.Error("entrypoint must preserve an existing or bind-mounted .zshrc")
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

func TestEntrypointDirectCodexExec(t *testing.T) {
	entrypoint, err := os.ReadFile("../../docker/sandbox/entrypoint.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(entrypoint)
	if !strings.Contains(script, `exec codex "${CODEX_ARGS[@]}" "${CODEX_EXTRA_ARGS[@]}"`) {
		t.Error("direct Codex path must exec argv without a shell")
	}
	if !strings.Contains(script, `json.loads(os.environ["DOCK_AGENT_ARGS_JSON"])`) {
		t.Error("direct Codex path must decode structured argv")
	}
	if strings.Contains(script, "sleep 1") {
		t.Error("entrypoint must not add an unconditional startup delay")
	}
}
