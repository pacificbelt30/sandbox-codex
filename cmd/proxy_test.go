package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveProxyDockerfile_ExplicitFlag(t *testing.T) {
	df, ctx, err := resolveProxyDockerfile("/tmp/custom/auth-proxy.Dockerfile")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if df != "/tmp/custom/auth-proxy.Dockerfile" {
		t.Fatalf("dockerfile = %q; want explicit path", df)
	}
	if ctx != "." {
		t.Fatalf("buildCtx = %q; want .", ctx)
	}
}

func TestResolveProxyDockerfile_RepoDockerfile(t *testing.T) {
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
		t.Fatalf("dockerfile = %q; want docker/auth-proxy.Dockerfile", df)
	}
	if ctx != "." {
		t.Fatalf("buildCtx = %q; want .", ctx)
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

	expected := filepath.Join(homeDir, ".config", "codex-dock", "auth-proxy.Dockerfile")
	if df != expected {
		t.Fatalf("dockerfile = %q; want %q", df, expected)
	}
	if ctx != "." {
		t.Fatalf("buildCtx = %q; want .", ctx)
	}

	data, err := os.ReadFile(expected)
	if err != nil {
		t.Fatalf("reading fallback dockerfile: %v", err)
	}
	if !strings.Contains(string(data), "ENTRYPOINT") {
		t.Fatalf("fallback dockerfile does not look valid")
	}
}
