package agentconfig

import (
	"strings"
	"testing"
)

func TestClaudeEnvironmentClearsCompetingCredentials(t *testing.T) {
	for _, oauth := range []bool{false, true} {
		env, err := ClaudeEnv("http://auth:18080/anthropic", "cdx-fixture", oauth)
		if err != nil {
			t.Fatal(err)
		}
		values := map[string]string{}
		for _, entry := range env {
			key, value, _ := strings.Cut(entry, "=")
			values[key] = value
		}
		selected, cleared := ClaudeAPIKeyEnv, ClaudeOAuthEnv
		if oauth {
			selected, cleared = cleared, selected
		}
		if values[selected] != "cdx-fixture" || values[cleared] != "" {
			t.Fatalf("wrong authentication selection: oauth=%t", oauth)
		}
		for _, key := range []string{"ANTHROPIC_AUTH_TOKEN", "CLAUDE_CODE_OAUTH_REFRESH_TOKEN", "CLAUDE_CODE_USE_BEDROCK", "CLAUDE_CODE_USE_VERTEX", "CLAUDE_CODE_USE_FOUNDRY"} {
			if value, ok := values[key]; !ok || value != "" {
				t.Fatalf("competing selector retained: %s", key)
			}
		}
	}
	for _, endpoint := range []string{"https://user:secret@auth/path", "file:///tmp/socket", "https://auth/path?token=secret", "http://auth/\nheader"} {
		if _, err := ClaudeEnv(endpoint, "cdx-fixture", false); err == nil {
			t.Fatal("invalid endpoint accepted")
		}
	}
}
