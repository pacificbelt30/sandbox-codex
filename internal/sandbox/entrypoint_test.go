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
