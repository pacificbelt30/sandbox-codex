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

func TestProxyContainerNames(t *testing.T) {
	// Auth proxy only (no http proxy configured).
	m := &Manager{proxy: &fakeProxy{endpoint: "http://codex-auth-proxy:18080"}}
	if got := m.proxyContainerNames(); len(got) != 1 || got[0] != "codex-auth-proxy" {
		t.Errorf("auth only: got %v; want [codex-auth-proxy]", got)
	}

	// Auth + http proxy → both names, in order, de-duplicated.
	m = &Manager{
		proxy:        &fakeProxy{endpoint: "http://codex-auth-proxy:18080/v1"},
		httpProxyURL: "http://codex-http-proxy:18082",
	}
	got := m.proxyContainerNames()
	if len(got) != 2 || got[0] != "codex-auth-proxy" || got[1] != "codex-http-proxy" {
		t.Errorf("auth+http: got %v; want [codex-auth-proxy codex-http-proxy]", got)
	}

	// No proxy configured → empty.
	if got := (&Manager{}).proxyContainerNames(); len(got) != 0 {
		t.Errorf("none: got %v; want empty", got)
	}
}

func TestBuildEnv_NoProxyExcludesProxyHostAndLoopback(t *testing.T) {
	m := newTestManager(&fakeProxy{endpoint: "http://codex-auth-proxy:18080", anthropic: true})
	env, err := m.buildEnv("w1", RunOptions{Agent: AgentNone, TokenTTL: 60}, "")
	if err != nil {
		t.Fatal(err)
	}
	// Both upper- and lower-case variants must be set for tools that read either.
	for _, key := range []string{"NO_PROXY", "no_proxy"} {
		v, ok := envValue(env, key)
		if !ok {
			t.Fatalf("%s not set", key)
		}
		for _, want := range []string{"codex-auth-proxy", "localhost", "127.0.0.1"} {
			if !strings.Contains(v, want) {
				t.Errorf("%s = %q; want it to contain %q", key, v, want)
			}
		}
	}
	// The lower-case proxy variants must also be present.
	if v, _ := envValue(env, "https_proxy"); v != "http://codex-auth-proxy:18080" {
		t.Errorf("https_proxy = %q; want http://codex-auth-proxy:18080", v)
	}
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
	// General egress is routed through the proxy/router via HTTP(S)_PROXY, with the
	// proxy host excluded from NO_PROXY so API/token traffic reaches it directly.
	if v, _ := envValue(env, "HTTPS_PROXY"); v != "http://codex-auth-proxy:18080" {
		t.Errorf("HTTPS_PROXY = %q; want http://codex-auth-proxy:18080", v)
	}
	if v, _ := envValue(env, "NO_PROXY"); !strings.Contains(v, "codex-auth-proxy") {
		t.Errorf("NO_PROXY = %q; want it to contain codex-auth-proxy", v)
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
	if v, _ := envValue(env, "HTTP_PROXY"); v != "http://codex-auth-proxy:18080" {
		t.Errorf("HTTP_PROXY = %q; want http://codex-auth-proxy:18080", v)
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

func TestBuildEnv_SeparateHTTPProxy(t *testing.T) {
	m := newTestManager(&fakeProxy{endpoint: "http://codex-auth-proxy:18080", anthropic: true})
	m.httpProxyURL = "http://codex-http-proxy:18082"
	env, err := m.buildEnv("w1", RunOptions{Agent: AgentNone, TokenTTL: 60}, "")
	if err != nil {
		t.Fatal(err)
	}
	// General egress → the dedicated egress proxy.
	if v, _ := envValue(env, "HTTP_PROXY"); v != "http://codex-http-proxy:18082" {
		t.Errorf("HTTP_PROXY = %q; want the egress proxy", v)
	}
	if v, _ := envValue(env, "https_proxy"); v != "http://codex-http-proxy:18082" {
		t.Errorf("https_proxy = %q; want the egress proxy", v)
	}
	// API still points at the auth proxy.
	if v, _ := envValue(env, "OPENAI_BASE_URL"); v != "http://codex-auth-proxy:18080/v1" {
		t.Errorf("OPENAI_BASE_URL = %q; want the auth proxy", v)
	}
	// NO_PROXY must exclude the AUTH host (so API/token go direct), not the egress host.
	if v, _ := envValue(env, "NO_PROXY"); !strings.Contains(v, "codex-auth-proxy") {
		t.Errorf("NO_PROXY = %q; want it to contain codex-auth-proxy", v)
	}
}

func TestBuildEnv_NoInternetSkipsProxyVars(t *testing.T) {
	m := newTestManager(&fakeProxy{endpoint: "http://codex-auth-proxy:18080", anthropic: true})
	env, err := m.buildEnv("w1", RunOptions{Agent: AgentCodex, TokenTTL: 60, NoInternet: true}, "")
	if err != nil {
		t.Fatal(err)
	}
	// API routes still configured...
	if _, ok := envValue(env, "OPENAI_BASE_URL"); !ok {
		t.Error("OPENAI_BASE_URL should still be set with --no-internet (API routes remain)")
	}
	// ...but general egress through the forward proxy is disabled.
	if _, ok := envValue(env, "HTTP_PROXY"); ok {
		t.Error("HTTP_PROXY should not be set when --no-internet is used")
	}
	if _, ok := envValue(env, "HTTPS_PROXY"); ok {
		t.Error("HTTPS_PROXY should not be set when --no-internet is used")
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
