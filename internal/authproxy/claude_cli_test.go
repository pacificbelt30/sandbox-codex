package authproxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pacificbelt30/codex-dock/internal/agentconfig"
)

// Used by both host CLI and real Kata acceptance. The final marker can only
// be reached after the official CLI executes Bash and returns its tool result.
func serveClaudeFixture(w http.ResponseWriter, r *http.Request, oauth bool) {
	if r.Header.Get("Anthropic-Version") != defaultAnthropicVersion {
		http.Error(w, "missing API version", 400)
		return
	}
	if oauth {
		if r.Header.Get("Authorization") != "Bearer fixture-anthropic-access" || r.Header.Get("X-Api-Key") != "" || !strings.Contains(r.Header.Get("Anthropic-Beta"), anthropicOAuthBetaHeader) {
			http.Error(w, "subscription credential mismatch", 400)
			return
		}
	} else if r.Header.Get("X-Api-Key") != "fixture-anthropic-key" || r.Header.Get("Authorization") != "" {
		http.Error(w, "API credential mismatch", 400)
		return
	}
	var body struct {
		Model    string `json:"model"`
		Stream   bool   `json:"stream"`
		Messages []struct {
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", 400)
		return
	}
	if r.URL.Path == "/v1/messages/count_tokens" {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"input_tokens":10}`))
		return
	}
	if r.URL.Path != "/v1/messages" || !body.Stream {
		http.Error(w, "expected streamed Messages", 400)
		return
	}
	toolDone := false
	for _, message := range body.Messages {
		var blocks []struct {
			Type    string          `json:"type"`
			Content json.RawMessage `json:"content"`
		}
		_ = json.Unmarshal(message.Content, &blocks)
		for _, block := range blocks {
			if block.Type == "tool_result" && strings.Contains(string(block.Content), "CXD_CLAUDE_TOOL_OK") {
				toolDone = true
			}
		}
	}
	w.Header().Set("Content-Type", "text/event-stream")
	event := func(kind string, fields map[string]any) {
		fields["type"] = kind
		encoded, _ := json.Marshal(fields)
		_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", kind, encoded)
		w.(http.Flusher).Flush()
	}
	event("message_start", map[string]any{"message": map[string]any{"id": "msg_claude_fixture", "type": "message", "role": "assistant", "model": body.Model, "content": []any{}, "stop_reason": nil, "stop_sequence": nil, "usage": map[string]int{"input_tokens": 10, "output_tokens": 0}}})
	stop := "tool_use"
	if toolDone {
		stop = "end_turn"
		event("content_block_start", map[string]any{"index": 0, "content_block": map[string]any{"type": "text", "text": ""}})
		event("content_block_delta", map[string]any{"index": 0, "delta": map[string]any{"type": "text_delta", "text": "CXD_CLAUDE_SSE_OK"}})
	} else {
		event("content_block_start", map[string]any{"index": 0, "content_block": map[string]any{"type": "tool_use", "id": "tool_claude_fixture", "name": "Bash", "input": map[string]any{}}})
		event("content_block_delta", map[string]any{"index": 0, "delta": map[string]any{"type": "input_json_delta", "partial_json": `{"command":"printf CXD_CLAUDE_TOOL_OK > cxd-claude-tool.txt; cat cxd-claude-tool.txt","description":"Write fixture marker"}`}})
	}
	event("content_block_stop", map[string]any{"index": 0})
	event("message_delta", map[string]any{"delta": map[string]any{"stop_reason": stop, "stop_sequence": nil}, "usage": map[string]int{"output_tokens": 5}})
	event("message_stop", map[string]any{})
}

func TestClaudeCLIIntegration(t *testing.T) {
	binary := os.Getenv("CXD_TEST_CLAUDE_BINARY")
	if binary == "" {
		t.Skip("set CXD_TEST_CLAUDE_BINARY to the pinned official CLI")
	}
	for _, oauth := range []bool{false, true} {
		t.Run(fmt.Sprintf("oauth=%t", oauth), func(t *testing.T) {
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { serveClaudeFixture(w, r, oauth) }))
			defer up.Close()
			p := newAnthropicAPIKeyProxy(t, "fixture-anthropic-key")
			p.cfg.AllowedModels = []string{"claude-sonnet-4-6"}
			if oauth {
				p.anthropicAPIKey = ""
				p.anthropicOAuth = &AnthropicOAuthCredentials{AccessToken: "fixture-anthropic-access"}
			}
			p.anthropicUpstreamURL = up.URL
			if err := p.Start(); err != nil {
				t.Fatal(err)
			}
			defer p.Stop()
			token, err := p.IssueToken("claude-fixture", 120)
			if err != nil {
				t.Fatal(err)
			}
			env, err := agentconfig.ClaudeEnv(p.Endpoint()+"/anthropic", token, oauth)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, binary, "--setting-sources", "", "-p", "Run the fixture command.", "--model", "claude-sonnet-4-6", "--tools", "Bash", "--allowedTools", "Bash", "--max-turns", "3")
			command.Dir = t.TempDir()
			command.Env = append(os.Environ(), env...)
			command.Env = append(command.Env, "HOME="+t.TempDir(), "CLAUDE_CONFIG_DIR="+t.TempDir(), "NO_PROXY=127.0.0.1,localhost", "no_proxy=127.0.0.1,localhost")
			var output bytes.Buffer
			command.Stdout = &output
			command.Stderr = &output
			if err := command.Run(); err != nil {
				t.Fatalf("official Claude CLI: %v\n%s", err, output.String())
			}
			if !strings.Contains(output.String(), "CXD_CLAUDE_SSE_OK") {
				t.Fatalf("missing final SSE response: %s", output.String())
			}
			marker, err := os.ReadFile(filepath.Join(command.Dir, "cxd-claude-tool.txt"))
			if err != nil || string(marker) != "CXD_CLAUDE_TOOL_OK" {
				t.Fatalf("tool not executed: %q %v", marker, err)
			}
		})
	}
}
