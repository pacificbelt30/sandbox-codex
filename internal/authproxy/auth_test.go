package authproxy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func withTempHome(t *testing.T) (string, func()) {
	t.Helper()
	dir := t.TempDir()
	orig := os.Getenv("HOME")
	os.Setenv("HOME", dir)
	return dir, func() { os.Setenv("HOME", orig) }
}

func TestSaveAndReadStoredKey(t *testing.T) {
	home, cleanup := withTempHome(t)
	defer cleanup()

	// Create config dir as SaveAPIKey would
	dir := filepath.Join(home, ".config", "codex-dock")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}

	const key = "sk-saved-key-abc123"
	if err := SaveAPIKey(key); err != nil {
		t.Fatalf("SaveAPIKey: %v", err)
	}

	got, err := readStoredKey()
	if err != nil {
		t.Fatalf("readStoredKey: %v", err)
	}
	if got != key {
		t.Errorf("readStoredKey = %q; want %q", got, key)
	}
}

func TestReadStoredKey_Missing(t *testing.T) {
	_, cleanup := withTempHome(t)
	defer cleanup()

	key, err := readStoredKey()
	if err == nil {
		t.Error("expected error for missing key file")
	}
	if key != "" {
		t.Errorf("expected empty key, got %q", key)
	}
}

func TestLoadAPIKey_EnvPriority(t *testing.T) {
	home, cleanup := withTempHome(t)
	defer cleanup()

	// Store a key in config
	dir := filepath.Join(home, ".config", "codex-dock")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	_ = SaveAPIKey("sk-stored-key")

	// Set env var — should take priority
	os.Setenv("OPENAI_API_KEY", "sk-env-key")
	defer os.Unsetenv("OPENAI_API_KEY")

	got := loadAPIKey()
	if got != "sk-env-key" {
		t.Errorf("loadAPIKey = %q; want sk-env-key (env var priority)", got)
	}
}

func TestLoadAPIKey_StoredFallback(t *testing.T) {
	home, cleanup := withTempHome(t)
	defer cleanup()

	os.Unsetenv("OPENAI_API_KEY")

	dir := filepath.Join(home, ".config", "codex-dock")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	_ = SaveAPIKey("sk-stored-only")

	got := loadAPIKey()
	if got != "sk-stored-only" {
		t.Errorf("loadAPIKey = %q; want sk-stored-only", got)
	}
}

func TestLoadAPIKey_CodexAuthJSON(t *testing.T) {
	home, cleanup := withTempHome(t)
	defer cleanup()

	os.Unsetenv("OPENAI_API_KEY")

	// Write a fake ~/.codex/auth.json
	codexDir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(codexDir, 0700); err != nil {
		t.Fatal(err)
	}
	authData, _ := json.Marshal(map[string]string{"OPENAI_API_KEY": "sk-from-auth-json"})
	if err := os.WriteFile(filepath.Join(codexDir, "auth.json"), authData, 0600); err != nil {
		t.Fatal(err)
	}

	got := loadAPIKey()
	if got != "sk-from-auth-json" {
		t.Errorf("loadAPIKey = %q; want sk-from-auth-json", got)
	}
}

func TestLoadAPIKey_None(t *testing.T) {
	_, cleanup := withTempHome(t)
	defer cleanup()
	os.Unsetenv("OPENAI_API_KEY")

	got := loadAPIKey()
	if got != "" {
		t.Errorf("loadAPIKey = %q; want empty when no key configured", got)
	}
}

func TestGetAuthInfo_Env(t *testing.T) {
	os.Setenv("OPENAI_API_KEY", "sk-test")
	defer os.Unsetenv("OPENAI_API_KEY")

	info, err := GetAuthInfo()
	if err != nil {
		t.Fatal(err)
	}
	if !info.KeyConfigured {
		t.Error("KeyConfigured should be true")
	}
	if info.Source != "OPENAI_API_KEY env" {
		t.Errorf("Source = %q; want 'OPENAI_API_KEY env'", info.Source)
	}
}

func TestGetAuthInfo_None(t *testing.T) {
	_, cleanup := withTempHome(t)
	defer cleanup()
	os.Unsetenv("OPENAI_API_KEY")

	info, err := GetAuthInfo()
	if err != nil {
		t.Fatal(err)
	}
	if info.KeyConfigured {
		t.Error("KeyConfigured should be false")
	}
	if info.Source != "none" {
		t.Errorf("Source = %q; want 'none'", info.Source)
	}
}

func TestReadCodexAuthJSON_MultipleKeys(t *testing.T) {
	home, cleanup := withTempHome(t)
	defer cleanup()

	codexDir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(codexDir, 0700); err != nil {
		t.Fatal(err)
	}

	// Test each key variant
	tests := []struct {
		jsonKey  string
		wantKey  string
	}{
		{"OPENAI_API_KEY", "sk-from-OPENAI_API_KEY"},
		{"api_key", "sk-from-api_key"},
		{"key", "sk-from-key"},
		{"token", "sk-from-token"},
	}

	for _, tt := range tests {
		t.Run(tt.jsonKey, func(t *testing.T) {
			data, _ := json.Marshal(map[string]string{tt.jsonKey: tt.wantKey})
			os.WriteFile(filepath.Join(codexDir, "auth.json"), data, 0600)

			got := readCodexAuthJSON()
			if got != tt.wantKey {
				t.Errorf("readCodexAuthJSON with key %q = %q; want %q", tt.jsonKey, got, tt.wantKey)
			}
		})
	}
}

func TestSaveAPIKey_FilePermissions(t *testing.T) {
	_, cleanup := withTempHome(t)
	defer cleanup()

	if err := SaveAPIKey("sk-perm-test"); err != nil {
		t.Fatalf("SaveAPIKey: %v", err)
	}

	home, _ := os.UserHomeDir()
	path := filepath.Join(home, ".config", "codex-dock", "apikey")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat apikey: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0600 {
		t.Errorf("apikey file mode = %o; want 0600", mode)
	}
}
