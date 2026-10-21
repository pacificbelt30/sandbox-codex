package authproxy

import (
	"os"
	"testing"
)

// Tests use fixtures exclusively. Never read the developer's real OAuth cache
// or API keys, including when an assertion would print a credential on failure.
func TestMain(m *testing.M) {
	if os.Getenv("CXD_KATA_FIXTURE_PROXY") == "1" {
		os.Exit(runKataFixtureProxy())
	}
	dir, err := os.MkdirTemp("", "codex-dock-auth-tests-")
	if err != nil {
		os.Exit(1)
	}
	for key, value := range map[string]string{"HOME": dir, "XDG_CONFIG_HOME": dir, "CODEX_HOME": dir, "OPENAI_API_KEY": "", "ANTHROPIC_API_KEY": "", "CLAUDE_CODE_OAUTH_TOKEN": ""} {
		if err := os.Setenv(key, value); err != nil {
			os.Exit(1)
		}
	}
	code := m.Run()
	if err := os.RemoveAll(dir); err != nil && code == 0 {
		code = 1
	}
	os.Exit(code)
}
