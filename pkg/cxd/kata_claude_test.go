package cxd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func verifyClaudeAPI(t *testing.T, ctx context.Context, c *Client, worker, project string, oauth bool, store *apiFixtureStore) {
	t.Helper()
	connections, err := c.Connections(ctx, worker)
	wantToken, wantHeader := "ANTHROPIC_API_KEY", "X-Api-Key"
	if oauth {
		wantToken, wantHeader = "CLAUDE_CODE_OAUTH_TOKEN", "Authorization"
	}
	if err != nil || len(connections) != 1 || connections[0].Provider != "anthropic" || connections[0].TokenEnvironment != wantToken || connections[0].AuthHeader != wantHeader {
		t.Fatalf("Claude connection: %+v %v", connections, err)
	}
	var version bytes.Buffer
	code, err := c.Exec(ctx, worker, ExecRequest{Argv: []string{"claude", "--version"}, Stdout: &version, Stderr: &version})
	if err != nil || code != 0 || strings.TrimSpace(version.String()) != "2.1.306 (Claude Code)" {
		t.Fatalf("pinned Claude CLI: %q %v", version.String(), err)
	}
	for invocation := 0; invocation < 2; invocation++ {
		if invocation > 0 {
			if err := c.Stop(ctx, worker, 5*time.Second); err != nil {
				t.Fatal(err)
			}
			if err := c.Start(ctx, worker); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(filepath.Join(project, "cxd-claude-tool.txt")); err != nil {
				t.Fatal(err)
			}
		}
		command, err := c.ClaudeCommand(ctx, worker)
		if err != nil {
			t.Fatal(err)
		}
		command.Argv = append(command.Argv, "-p", "Run the fixture command.", "--model", "claude-sonnet-4-6", "--tools", "Bash", "--allowedTools", "Bash", "--max-turns", "3")
		command.Env = append(command.Env, "CLAUDE_CONFIG_DIR=/var/tmp/claude-independent-config")
		var output bytes.Buffer
		callCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		code, err = c.Exec(callCtx, worker, ExecRequest{Argv: command.Argv, Env: command.Env, Stdout: &output, Stderr: &output})
		cancel()
		if err != nil || code != 0 || !strings.Contains(output.String(), "CXD_CLAUDE_SSE_OK") {
			t.Fatalf("Claude Messages/SSE: code=%d err=%v output=%s", code, err, output.String())
		}
		marker, err := os.ReadFile(filepath.Join(project, "cxd-claude-tool.txt"))
		if err != nil || string(marker) != "CXD_CLAUDE_TOOL_OK" {
			t.Fatalf("Claude tool execution: %q %v", marker, err)
		}
	}
	if oauth {
		store.mu.Lock()
		defer store.mu.Unlock()
		if store.saves != 1 || store.creds.Version != "version-1-next" || store.creds.ConnectionVersion != "connection-1" || store.creds.AnthropicOAuth.AccessToken != "fixture-anthropic-access" || store.creds.AnthropicOAuth.RefreshToken != "fixture-anthropic-refresh-rotated" {
			t.Fatal("Claude refresh was not persisted once without changing the connection generation")
		}
	}
	t.Log("verified pinned official Claude CLI: Messages/SSE, Bash tool round trip, API/subscription selection and token restore after VM restart")
}
