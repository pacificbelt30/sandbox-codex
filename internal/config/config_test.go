package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := defaultConfig()

	if cfg.DefaultImage != "codex-dock:latest" {
		t.Errorf("DefaultImage = %q; want codex-dock:latest", cfg.DefaultImage)
	}
	if cfg.DefaultTokenTTL != 3600 {
		t.Errorf("DefaultTokenTTL = %d; want 3600", cfg.DefaultTokenTTL)
	}
	if cfg.NetworkName != "dock-net" {
		t.Errorf("NetworkName = %q; want dock-net", cfg.NetworkName)
	}
}

func TestLoad_Defaults(t *testing.T) {
	// Use a temp home with no config file
	dir := t.TempDir()
	orig := os.Getenv("HOME")
	os.Setenv("HOME", dir)
	defer os.Setenv("HOME", orig)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DefaultImage != "codex-dock:latest" {
		t.Errorf("DefaultImage = %q; want codex-dock:latest", cfg.DefaultImage)
	}
	if cfg.DefaultTokenTTL != 3600 {
		t.Errorf("DefaultTokenTTL = %d; want 3600", cfg.DefaultTokenTTL)
	}
	if cfg.NetworkName != "dock-net" {
		t.Errorf("NetworkName = %q; want dock-net", cfg.NetworkName)
	}
}

func TestLoad_FromFile(t *testing.T) {
	dir := t.TempDir()
	orig := os.Getenv("HOME")
	os.Setenv("HOME", dir)
	defer os.Setenv("HOME", orig)

	cfgDir := filepath.Join(dir, ".config", "codex-dock")
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatal(err)
	}

	toml := `
default_image = "my-custom:v1"
default_token_ttl = 1800
network_name = "my-net"
verbose = true
`
	if err := os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte(toml), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DefaultImage != "my-custom:v1" {
		t.Errorf("DefaultImage = %q; want my-custom:v1", cfg.DefaultImage)
	}
	if cfg.DefaultTokenTTL != 1800 {
		t.Errorf("DefaultTokenTTL = %d; want 1800", cfg.DefaultTokenTTL)
	}
	if cfg.NetworkName != "my-net" {
		t.Errorf("NetworkName = %q; want my-net", cfg.NetworkName)
	}
	if !cfg.Verbose {
		t.Error("Verbose should be true")
	}
}
