package sandbox

import (
	"strings"
	"testing"
)

// fakeProxy implements authproxy.Service for env-building tests.
type fakeProxy struct {
	oauth     bool
	anthropic bool
	endpoint  string
	issued    []string
}

func (f *fakeProxy) IssueToken(name string, ttlSec int) (string, error) {
	f.issued = append(f.issued, name)
	return "cdx-faketoken", nil
}
func (f *fakeProxy) RevokeToken(name string)   {}
func (f *fakeProxy) IsOAuthMode() bool         { return f.oauth }
func (f *fakeProxy) IsAnthropicMode() bool     { return f.anthropic }
func (f *fakeProxy) ContainerEndpoint() string { return f.endpoint }

// envValue returns the value of key=... in env, or ("", false) if absent.
func envValue(env []string, key string) (string, bool) {
	prefix := key + "="
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			return strings.TrimPrefix(e, prefix), true
		}
	}
	return "", false
}

func newTestManager(p *fakeProxy) *Manager {
	return &Manager{proxy: p}
}

func TestBuildEnv_AlwaysSetsAgentAndSandbox(t *testing.T) {
	m := newTestManager(&fakeProxy{endpoint: "http://codex-auth-proxy:18080", anthropic: true})
	env, err := m.buildEnv("w1", RunOptions{Agent: AgentCodex, TokenTTL: 60}, "")
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := envValue(env, "DOCK_AGENT"); v != "codex" {
		t.Errorf("DOCK_AGENT = %q; want codex", v)
	}
	if _, ok := envValue(env, "CODEX_SANDBOX"); !ok {
		t.Error("CODEX_SANDBOX not set")
	}
}

func TestBuildEnv_Codex(t *testing.T) {
	m := newTestManager(&fakeProxy{endpoint: "http://codex-auth-proxy:18080", anthropic: true})
	env, err := m.buildEnv("w1", RunOptions{Agent: AgentCodex, TokenTTL: 60}, "")
	if err != nil {
		t.Fatal(err)
	}

	if v, _ := envValue(env, "CODEX_TOKEN"); v != "cdx-faketoken" {
		t.Errorf("CODEX_TOKEN = %q; want cdx-faketoken", v)
	}
	if v, _ := envValue(env, "OPENAI_BASE_URL"); v != "http://codex-auth-proxy:18080/v1" {
		t.Errorf("OPENAI_BASE_URL = %q", v)
	}
	if _, ok := envValue(env, "CODEX_AUTH_PROXY_FALLBACK_URLS"); !ok {
		t.Error("expected CODEX_AUTH_PROXY_FALLBACK_URLS for codex-auth-proxy endpoint")
	}
	// Codex agent must not receive Anthropic variables.
	if _, ok := envValue(env, "ANTHROPIC_BASE_URL"); ok {
		t.Error("ANTHROPIC_BASE_URL should not be set for --agent codex")
	}
}

func TestBuildEnv_CodexOAuthRefreshOverride(t *testing.T) {
	m := newTestManager(&fakeProxy{endpoint: "http://codex-auth-proxy:18080", oauth: true})
	env, err := m.buildEnv("w1", RunOptions{Agent: AgentCodex, TokenTTL: 60}, "")
	if err != nil {
		t.Fatal(err)
	}
	v, ok := envValue(env, "CODEX_REFRESH_TOKEN_URL_OVERRIDE")
	if !ok {
		t.Fatal("CODEX_REFRESH_TOKEN_URL_OVERRIDE not set in OAuth mode")
	}
	if !strings.Contains(v, "/oauth/token?cdx=cdx-faketoken") {
		t.Errorf("refresh override = %q; want it to embed the cdx token", v)
	}
}

func TestBuildEnv_Claude(t *testing.T) {
	m := newTestManager(&fakeProxy{endpoint: "http://codex-auth-proxy:18080", anthropic: true})
	env, err := m.buildEnv("w1", RunOptions{Agent: AgentClaude, TokenTTL: 60}, "")
	if err != nil {
		t.Fatal(err)
	}

	if v, _ := envValue(env, "ANTHROPIC_BASE_URL"); v != "http://codex-auth-proxy:18080/anthropic" {
		t.Errorf("ANTHROPIC_BASE_URL = %q", v)
	}
	if v, _ := envValue(env, "ANTHROPIC_API_KEY"); v != "cdx-faketoken" {
		t.Errorf("ANTHROPIC_API_KEY = %q; want placeholder token", v)
	}
	if _, ok := envValue(env, "ANTHROPIC_PROXY_FALLBACK_URLS"); !ok {
		t.Error("expected ANTHROPIC_PROXY_FALLBACK_URLS")
	}
	// Claude agent must not receive Codex variables.
	if _, ok := envValue(env, "CODEX_TOKEN"); ok {
		t.Error("CODEX_TOKEN should not be set for --agent claude")
	}
	if _, ok := envValue(env, "OPENAI_BASE_URL"); ok {
		t.Error("OPENAI_BASE_URL should not be set for --agent claude")
	}
}

func TestBuildEnv_ClaudeWithoutAnthropicCreds(t *testing.T) {
	m := newTestManager(&fakeProxy{endpoint: "http://codex-auth-proxy:18080", anthropic: false})
	env, err := m.buildEnv("w1", RunOptions{Agent: AgentClaude, TokenTTL: 60}, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := envValue(env, "ANTHROPIC_BASE_URL"); ok {
		t.Error("ANTHROPIC_BASE_URL should be omitted when the proxy has no Anthropic creds")
	}
}

func TestBuildEnv_ShellSetsBothProviders(t *testing.T) {
	m := newTestManager(&fakeProxy{endpoint: "http://codex-auth-proxy:18080", anthropic: true})
	env, err := m.buildEnv("w1", RunOptions{Agent: AgentNone, TokenTTL: 60}, "")
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := envValue(env, "DOCK_AGENT"); v != "" {
		t.Errorf("DOCK_AGENT = %q; want empty for shell", v)
	}
	if _, ok := envValue(env, "CODEX_TOKEN"); !ok {
		t.Error("shell mode should configure Codex auth")
	}
	if _, ok := envValue(env, "ANTHROPIC_BASE_URL"); !ok {
		t.Error("shell mode should configure Claude auth when available")
	}
}

func TestBuildEnv_NoProxy(t *testing.T) {
	m := &Manager{}
	env, err := m.buildEnv("w1", RunOptions{Agent: AgentCodex, TokenTTL: 60}, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := envValue(env, "CODEX_TOKEN"); ok {
		t.Error("CODEX_TOKEN should not be set without a proxy")
	}
	if v, _ := envValue(env, "DOCK_AGENT"); v != "codex" {
		t.Errorf("DOCK_AGENT = %q; want codex", v)
	}
}

func TestValidAgent(t *testing.T) {
	for _, a := range []Agent{AgentNone, AgentCodex, AgentClaude} {
		if !ValidAgent(a) {
			t.Errorf("ValidAgent(%q) = false; want true", a)
		}
	}
	if ValidAgent(Agent("bogus")) {
		t.Error("ValidAgent(bogus) = true; want false")
	}
}
