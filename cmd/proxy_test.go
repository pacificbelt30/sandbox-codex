package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
	args := buildProxyRunArgs("my-proxy", 18080, "codex-dock-proxy:latest",
		"0.0.0.0:18080", "", "", "", "")

	assertContainsSequence(t, args, "run", "-d", "--name", "my-proxy", "-p", "18080:18080")
	assertContainsSequence(t, args, "codex-dock-proxy:latest", "proxy", "serve", "--listen", "0.0.0.0:18080")

	// No credential flags when nothing is set.
	for _, a := range args {
		if strings.HasPrefix(a, "OPENAI_API_KEY") {
			t.Errorf("unexpected OPENAI_API_KEY in args: %v", args)
		}
		if strings.Contains(a, "apikey") {
			t.Errorf("unexpected apikey mount in args: %v", args)
		}
		if strings.Contains(a, "auth.json") {
			t.Errorf("unexpected auth.json mount in args: %v", args)
		}
	}
}

func TestBuildProxyRunArgs_APIKeyEnv(t *testing.T) {
	args := buildProxyRunArgs("proxy", 18080, "img:latest", "0.0.0.0:18080", "",
		"sk-test-key", "", "")

	if !containsSequence(args, "-e", "OPENAI_API_KEY=sk-test-key") {
		t.Errorf("expected -e OPENAI_API_KEY=sk-test-key in args: %v", args)
	}
}

func TestBuildProxyRunArgs_StoredKeyMount(t *testing.T) {
	keyFile := filepath.Join(t.TempDir(), "apikey")
	if err := os.WriteFile(keyFile, []byte(`{"key":"stored"}`), 0600); err != nil {
		t.Fatal(err)
	}

	args := buildProxyRunArgs("proxy", 18080, "img:latest", "0.0.0.0:18080", "",
		"", keyFile, "")

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

	args := buildProxyRunArgs("proxy", 18080, "img:latest", "0.0.0.0:18080", "",
		"", "", authFile)

	wantMount := authFile + ":/root/.codex/auth.json:ro"
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

	args := buildProxyRunArgs("proxy", 18080, "img:latest", "0.0.0.0:18080", "",
		"sk-env-key", keyFile, authFile)

	if !containsSequence(args, "-e", "OPENAI_API_KEY=sk-env-key") {
		t.Errorf("missing env key in args: %v", args)
	}
	if !containsSequence(args, "-v", keyFile+":/root/.config/codex-dock/apikey:ro") {
		t.Errorf("missing stored key mount in args: %v", args)
	}
	if !containsSequence(args, "-v", authFile+":/root/.codex/auth.json:ro") {
		t.Errorf("missing auth.json mount in args: %v", args)
	}
}

func TestBuildProxyRunArgs_AdminSecret(t *testing.T) {
	args := buildProxyRunArgs("proxy", 18080, "img:latest", "0.0.0.0:18080",
		"s3cr3t", "", "", "")

	if !containsSequence(args, "--admin-secret", "s3cr3t") {
		t.Errorf("expected --admin-secret s3cr3t in args: %v", args)
	}
}

func TestBuildProxyRunArgs_NoAdminSecret(t *testing.T) {
	args := buildProxyRunArgs("proxy", 18080, "img:latest", "0.0.0.0:18080",
		"", "", "", "")

	for _, a := range args {
		if a == "--admin-secret" {
			t.Errorf("unexpected --admin-secret in args when adminSecret is empty: %v", args)
		}
	}
}

func TestBuildProxyRunArgs_MissingFiles(t *testing.T) {
	// Non-existent paths must not produce volume mounts.
	args := buildProxyRunArgs("proxy", 18080, "img:latest", "0.0.0.0:18080", "",
		"", "/nonexistent/apikey", "/nonexistent/auth.json")

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
