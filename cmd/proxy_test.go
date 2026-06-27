package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pacificbelt30/codex-dock/internal/authproxy"
	"github.com/spf13/viper"
)

// TestInitConfigDefaults ensures the proxy image (and other settings read via
// viper) have a built-in default even with no config file or env override, so
// `proxy run`/`proxy build` don't fail with an empty image name.
func TestInitConfigDefaults(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	initConfig()

	if got := viper.GetString("proxy_image"); got != "codex-dock-proxy:latest" {
		t.Errorf("proxy_image default = %q; want codex-dock-proxy:latest", got)
	}
	if got := viper.GetString("default_image"); got != "codex-dock:latest" {
		t.Errorf("default_image default = %q; want codex-dock:latest", got)
	}
	if got := viper.GetInt("default_token_ttl"); got != 3600 {
		t.Errorf("default_token_ttl default = %d; want 3600", got)
	}
}

// TestBuildProxyRunArgs_AdminBindEgress checks that `proxy run` binds the admin
// listener to the egress sentinel (so workers cannot reach it) while publishing
// only the admin port to host loopback.
func TestBuildProxyRunArgs_AdminBindEgress(t *testing.T) {
	args := buildProxyRunArgs(proxyRunArgs{
		name: "p", adminPort: 18081, networkName: "dock-net-proxy", image: "img",
		listenAddr: "0.0.0.0:18080", adminListenAddr: fmt.Sprintf("%s:18081", authproxy.AdminBindEgress),
	})
	if !containsSequence(args, "--admin-listen", "egress:18081") {
		t.Errorf("expected --admin-listen egress:18081 in args: %v", args)
	}
	if !containsSequence(args, "-p", "127.0.0.1:18081:18081") {
		t.Errorf("expected admin port published to loopback only: %v", args)
	}
	// The data-plane port must NOT be published to the host.
	for _, a := range args {
		if a == "18080:18080" || a == "0.0.0.0:18080:18080" || a == "127.0.0.1:18080:18080" {
			t.Errorf("data-plane port should not be published: %v", args)
		}
	}
}

// ---- resolveProxyDockerfile -----------------------------------------------

func TestResolveProxyDockerfile_ExplicitFlag(t *testing.T) {
	df, ctx, err := resolveProxyDockerfile("/custom/auth-proxy.Dockerfile")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if df != "/custom/auth-proxy.Dockerfile" {
		t.Errorf("dockerfile = %q; want /custom/auth-proxy.Dockerfile", df)
	}
	if ctx != "." {
		t.Errorf("buildCtx = %q; want .", ctx)
	}
}

func TestResolveProxyDockerfile_CWDDockerfile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "auth-proxy.Dockerfile"), []byte("FROM scratch\n"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	df, ctx, err := resolveProxyDockerfile("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if df != "auth-proxy.Dockerfile" {
		t.Errorf("dockerfile = %q; want auth-proxy.Dockerfile", df)
	}
	if ctx != "." {
		t.Errorf("buildCtx = %q; want .", ctx)
	}
}

func TestResolveProxyDockerfile_DockerSubdir(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docker"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docker", "auth-proxy.Dockerfile"), []byte("FROM scratch\n"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	df, ctx, err := resolveProxyDockerfile("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if df != "docker/auth-proxy.Dockerfile" {
		t.Errorf("dockerfile = %q; want docker/auth-proxy.Dockerfile", df)
	}
	if ctx != "." {
		t.Errorf("buildCtx = %q; want .", ctx)
	}
}

func TestResolveProxyDockerfile_FallbackToConfigDir(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	df, ctx, err := resolveProxyDockerfile("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx != "." {
		t.Errorf("buildCtx = %q; want .", ctx)
	}
	if !strings.HasSuffix(df, "auth-proxy.Dockerfile") {
		t.Errorf("dockerfile = %q; expected to end with auth-proxy.Dockerfile", df)
	}

	configDir := filepath.Join(homeDir, ".config", "codex-dock")
	if _, err := os.Stat(filepath.Join(configDir, "auth-proxy.Dockerfile")); err != nil {
		t.Errorf("auth-proxy.Dockerfile not written to config dir: %v", err)
	}
}

// ---- ensureProxyDockerfile ------------------------------------------------

func TestEnsureProxyDockerfile_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	if err := ensureProxyDockerfile(dir); err != nil {
		t.Fatalf("ensureProxyDockerfile: %v", err)
	}

	info, err := os.Stat(filepath.Join(dir, "auth-proxy.Dockerfile"))
	if err != nil {
		t.Fatalf("auth-proxy.Dockerfile not created: %v", err)
	}
	if info.Size() == 0 {
		t.Error("auth-proxy.Dockerfile is empty")
	}
}

func TestEnsureProxyDockerfile_Idempotent(t *testing.T) {
	dir := t.TempDir()
	if err := ensureProxyDockerfile(dir); err != nil {
		t.Fatalf("first call: %v", err)
	}

	const custom = "# custom proxy Dockerfile\n"
	if err := os.WriteFile(filepath.Join(dir, "auth-proxy.Dockerfile"), []byte(custom), 0644); err != nil {
		t.Fatal(err)
	}

	if err := ensureProxyDockerfile(dir); err != nil {
		t.Fatalf("second call: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "auth-proxy.Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != custom {
		t.Errorf("existing file was overwritten; got %q; want %q", string(data), custom)
	}
}

func TestEnsureProxyDockerfile_Content(t *testing.T) {
	dir := t.TempDir()
	if err := ensureProxyDockerfile(dir); err != nil {
		t.Fatalf("ensureProxyDockerfile: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "auth-proxy.Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, marker := range []string{"FROM golang:", "go build", "ENTRYPOINT"} {
		if !strings.Contains(content, marker) {
			t.Errorf("auth-proxy.Dockerfile missing expected content %q", marker)
		}
	}
}

// ---- buildProxyRunArgs ----------------------------------------------------

func TestBuildProxyRunArgs_Basic(t *testing.T) {
	args := buildProxyRunArgs(proxyRunArgs{
		name: "my-proxy", adminPort: 18081, networkName: "dock-net-proxy",
		image: "codex-dock-proxy:latest", listenAddr: "0.0.0.0:18080", adminListenAddr: "0.0.0.0:18081",
	})

	// Only the admin port is published (host loopback); the data-plane port is internal.
	assertContainsSequence(t, args, "run", "-d", "--name", "my-proxy", "--network", "dock-net-proxy", "-p", "127.0.0.1:18081:18081")
	assertContainsSequence(t, args, "codex-dock-proxy:latest", "proxy", "serve", "--listen", "0.0.0.0:18080", "--admin-listen", "0.0.0.0:18081")

	// No credential flags when nothing is set.
	for _, a := range args {
		if strings.HasPrefix(a, "OPENAI_API_KEY") || strings.HasPrefix(a, "ANTHROPIC_API_KEY") {
			t.Errorf("unexpected API key env in args: %v", args)
		}
		if strings.Contains(a, "apikey") {
			t.Errorf("unexpected apikey mount in args: %v", args)
		}
		if strings.Contains(a, "auth.json") || strings.Contains(a, ".credentials.json") {
			t.Errorf("unexpected credential mount in args: %v", args)
		}
	}
}

func TestBuildProxyRunArgs_APIKeyEnv(t *testing.T) {
	args := buildProxyRunArgs(proxyRunArgs{
		name: "proxy", adminPort: 18081, networkName: "dock-net-proxy", image: "img:latest",
		listenAddr: "0.0.0.0:18080", apiKeyEnv: "sk-test-key",
	})

	if !containsSequence(args, "-e", "OPENAI_API_KEY=sk-test-key") {
		t.Errorf("expected -e OPENAI_API_KEY=sk-test-key in args: %v", args)
	}
}

func TestBuildProxyRunArgs_AnthropicKeyEnv(t *testing.T) {
	args := buildProxyRunArgs(proxyRunArgs{
		name: "proxy", adminPort: 18081, networkName: "dock-net-proxy", image: "img:latest",
		listenAddr: "0.0.0.0:18080", anthropicKeyEnv: "sk-ant-test",
	})

	if !containsSequence(args, "-e", "ANTHROPIC_API_KEY=sk-ant-test") {
		t.Errorf("expected -e ANTHROPIC_API_KEY=sk-ant-test in args: %v", args)
	}
}

func TestBuildProxyRunArgs_StoredKeyMount(t *testing.T) {
	keyFile := filepath.Join(t.TempDir(), "apikey")
	if err := os.WriteFile(keyFile, []byte(`{"key":"stored"}`), 0600); err != nil {
		t.Fatal(err)
	}

	args := buildProxyRunArgs(proxyRunArgs{
		name: "proxy", adminPort: 18081, networkName: "dock-net-proxy", image: "img:latest",
		listenAddr: "0.0.0.0:18080", storedKeyPath: keyFile,
	})

	wantMount := keyFile + ":/root/.config/codex-dock/apikey:ro"
	if !containsSequence(args, "-v", wantMount) {
		t.Errorf("expected -v %s in args: %v", wantMount, args)
	}
}

func TestBuildProxyRunArgs_OAuthMount(t *testing.T) {
	authFile := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(authFile, []byte(`{"access_token":"tok"}`), 0600); err != nil {
		t.Fatal(err)
	}

	args := buildProxyRunArgs(proxyRunArgs{
		name: "proxy", adminPort: 18081, networkName: "dock-net-proxy", image: "img:latest",
		listenAddr: "0.0.0.0:18080", oauthJSONPath: authFile,
	})

	wantMount := authFile + ":/root/.codex/auth.json:ro"
	if !containsSequence(args, "-v", wantMount) {
		t.Errorf("expected -v %s in args: %v", wantMount, args)
	}
}

func TestBuildProxyRunArgs_ClaudeCredsMount(t *testing.T) {
	credsFile := filepath.Join(t.TempDir(), ".credentials.json")
	if err := os.WriteFile(credsFile, []byte(`{"claudeAiOauth":{"accessToken":"x"}}`), 0600); err != nil {
		t.Fatal(err)
	}

	args := buildProxyRunArgs(proxyRunArgs{
		name: "proxy", adminPort: 18081, networkName: "dock-net-proxy", image: "img:latest",
		listenAddr: "0.0.0.0:18080", claudeCredsPath: credsFile,
	})

	wantMount := credsFile + ":/root/.claude/.credentials.json:ro"
	if !containsSequence(args, "-v", wantMount) {
		t.Errorf("expected -v %s in args: %v", wantMount, args)
	}
}

func TestBuildProxyRunArgs_AllCredentials(t *testing.T) {
	keyFile := filepath.Join(t.TempDir(), "apikey")
	if err := os.WriteFile(keyFile, []byte(`{"key":"stored"}`), 0600); err != nil {
		t.Fatal(err)
	}
	authFile := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(authFile, []byte(`{"access_token":"tok"}`), 0600); err != nil {
		t.Fatal(err)
	}
	anthropicKeyFile := filepath.Join(t.TempDir(), "anthropic-apikey")
	if err := os.WriteFile(anthropicKeyFile, []byte(`{"key":"sk-ant"}`), 0600); err != nil {
		t.Fatal(err)
	}
	credsFile := filepath.Join(t.TempDir(), ".credentials.json")
	if err := os.WriteFile(credsFile, []byte(`{"claudeAiOauth":{"accessToken":"x"}}`), 0600); err != nil {
		t.Fatal(err)
	}

	args := buildProxyRunArgs(proxyRunArgs{
		name: "proxy", adminPort: 18081, networkName: "dock-net-proxy", image: "img:latest",
		listenAddr:       "0.0.0.0:18080",
		apiKeyEnv:        "sk-env-key",
		storedKeyPath:    keyFile,
		oauthJSONPath:    authFile,
		anthropicKeyEnv:  "sk-ant-env",
		anthropicKeyPath: anthropicKeyFile,
		claudeCredsPath:  credsFile,
	})

	if !containsSequence(args, "-e", "OPENAI_API_KEY=sk-env-key") {
		t.Errorf("missing OpenAI env key in args: %v", args)
	}
	if !containsSequence(args, "-e", "ANTHROPIC_API_KEY=sk-ant-env") {
		t.Errorf("missing Anthropic env key in args: %v", args)
	}
	if !containsSequence(args, "-v", keyFile+":/root/.config/codex-dock/apikey:ro") {
		t.Errorf("missing stored key mount in args: %v", args)
	}
	if !containsSequence(args, "-v", anthropicKeyFile+":/root/.config/codex-dock/anthropic-apikey:ro") {
		t.Errorf("missing anthropic key mount in args: %v", args)
	}
	if !containsSequence(args, "-v", authFile+":/root/.codex/auth.json:ro") {
		t.Errorf("missing auth.json mount in args: %v", args)
	}
	if !containsSequence(args, "-v", credsFile+":/root/.claude/.credentials.json:ro") {
		t.Errorf("missing claude creds mount in args: %v", args)
	}
}

func TestBuildProxyRunArgs_AdminSecret(t *testing.T) {
	args := buildProxyRunArgs(proxyRunArgs{
		name: "proxy", adminPort: 18081, networkName: "dock-net-proxy", image: "img:latest",
		listenAddr: "0.0.0.0:18080", adminSecret: "s3cr3t",
	})

	if !containsSequence(args, "--admin-secret", "s3cr3t") {
		t.Errorf("expected --admin-secret s3cr3t in args: %v", args)
	}
}

func TestBuildProxyRunArgs_NoAdminSecret(t *testing.T) {
	args := buildProxyRunArgs(proxyRunArgs{
		name: "proxy", adminPort: 18081, networkName: "dock-net-proxy", image: "img:latest",
		listenAddr: "0.0.0.0:18080",
	})

	for _, a := range args {
		if a == "--admin-secret" {
			t.Errorf("unexpected --admin-secret in args when adminSecret is empty: %v", args)
		}
	}
}

func TestBuildProxyRunArgs_MissingFiles(t *testing.T) {
	// Non-existent paths must not produce volume mounts.
	args := buildProxyRunArgs(proxyRunArgs{
		name: "proxy", adminPort: 18081, networkName: "dock-net-proxy", image: "img:latest",
		listenAddr:       "0.0.0.0:18080",
		storedKeyPath:    "/nonexistent/apikey",
		oauthJSONPath:    "/nonexistent/auth.json",
		anthropicKeyPath: "/nonexistent/anthropic-apikey",
		claudeCredsPath:  "/nonexistent/.credentials.json",
	})

	for _, a := range args {
		if strings.Contains(a, "nonexistent") {
			t.Errorf("non-existent path appeared in args: %v", args)
		}
	}
}

// ---- helpers --------------------------------------------------------------

// containsSequence reports whether needle (in order) appears as a contiguous
// subsequence within haystack.
func containsSequence(haystack []string, needle ...string) bool {
	if len(needle) == 0 {
		return true
	}
outer:
	for i := range haystack {
		if i+len(needle) > len(haystack) {
			break
		}
		for j, n := range needle {
			if haystack[i+j] != n {
				continue outer
			}
		}
		return true
	}
	return false
}

// assertContainsSequence is like containsSequence but calls t.Errorf on failure.
func assertContainsSequence(t *testing.T, haystack []string, needle ...string) {
	t.Helper()
	if !containsSequence(haystack, needle...) {
		t.Errorf("expected sequence %v in args %v", needle, haystack)
	}
}
