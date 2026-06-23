package authproxy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeClaudeCreds(t *testing.T, home string, oauth map[string]interface{}) {
	t.Helper()
	dir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(map[string]interface{}{"claudeAiOauth": oauth})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".credentials.json"), b, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadAnthropicOAuthCredentials_Valid(t *testing.T) {
	home, cleanup := withTempHome(t)
	defer cleanup()

	writeClaudeCreds(t, home, map[string]interface{}{
		"accessToken":      "oat-access",
		"refreshToken":     "oat-refresh",
		"expiresAt":        int64(1893456000000),
		"scopes":           []string{"user:inference"},
		"subscriptionType": "max",
	})

	creds, err := LoadAnthropicOAuthCredentials()
	if err != nil {
		t.Fatalf("LoadAnthropicOAuthCredentials: %v", err)
	}
	if creds.AccessToken != "oat-access" {
		t.Errorf("AccessToken = %q; want oat-access", creds.AccessToken)
	}
	if creds.RefreshToken != "oat-refresh" {
		t.Errorf("RefreshToken = %q; want oat-refresh", creds.RefreshToken)
	}
	if creds.ExpiresAt != 1893456000000 {
		t.Errorf("ExpiresAt = %d; want 1893456000000", creds.ExpiresAt)
	}
	if creds.SubscriptionType != "max" {
		t.Errorf("SubscriptionType = %q; want max", creds.SubscriptionType)
	}
}

func TestLoadAnthropicOAuthCredentials_NoFile(t *testing.T) {
	_, cleanup := withTempHome(t)
	defer cleanup()

	if _, err := LoadAnthropicOAuthCredentials(); err == nil {
		t.Error("expected error when .credentials.json does not exist")
	}
}

func TestLoadAnthropicOAuthCredentials_MissingAccessToken(t *testing.T) {
	home, cleanup := withTempHome(t)
	defer cleanup()

	writeClaudeCreds(t, home, map[string]interface{}{"refreshToken": "rt-only"})

	if _, err := LoadAnthropicOAuthCredentials(); err == nil {
		t.Error("expected error when accessToken is missing")
	}
}

func TestIsAnthropicOAuth(t *testing.T) {
	home, cleanup := withTempHome(t)
	defer cleanup()

	if IsAnthropicOAuth() {
		t.Error("IsAnthropicOAuth() = true; want false with no credentials file")
	}

	writeClaudeCreds(t, home, map[string]interface{}{"accessToken": "oat"})
	if !IsAnthropicOAuth() {
		t.Error("IsAnthropicOAuth() = false; want true when accessToken present")
	}
}

func TestLoadAnthropicAPIKey_EnvPriority(t *testing.T) {
	home, cleanup := withTempHome(t)
	defer cleanup()

	dir := filepath.Join(home, ".config", "codex-dock")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := SaveAnthropicAPIKey("sk-ant-stored"); err != nil {
		t.Fatal(err)
	}

	os.Setenv("ANTHROPIC_API_KEY", "sk-ant-env")
	defer os.Unsetenv("ANTHROPIC_API_KEY")

	if got := loadAnthropicAPIKey(); got != "sk-ant-env" {
		t.Errorf("loadAnthropicAPIKey = %q; want sk-ant-env (env priority)", got)
	}
}

func TestLoadAnthropicAPIKey_StoredFallback(t *testing.T) {
	_, cleanup := withTempHome(t)
	defer cleanup()
	os.Unsetenv("ANTHROPIC_API_KEY")

	if err := SaveAnthropicAPIKey("sk-ant-only"); err != nil {
		t.Fatal(err)
	}

	if got := loadAnthropicAPIKey(); got != "sk-ant-only" {
		t.Errorf("loadAnthropicAPIKey = %q; want sk-ant-only", got)
	}
}

func TestLoadAnthropicAPIKey_None(t *testing.T) {
	_, cleanup := withTempHome(t)
	defer cleanup()
	os.Unsetenv("ANTHROPIC_API_KEY")

	if got := loadAnthropicAPIKey(); got != "" {
		t.Errorf("loadAnthropicAPIKey = %q; want empty", got)
	}
}

func TestSaveAnthropicAPIKey_FilePermissions(t *testing.T) {
	_, cleanup := withTempHome(t)
	defer cleanup()

	if err := SaveAnthropicAPIKey("sk-ant-perm"); err != nil {
		t.Fatalf("SaveAnthropicAPIKey: %v", err)
	}

	home, _ := os.UserHomeDir()
	path := filepath.Join(home, ".config", "codex-dock", "anthropic-apikey")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat anthropic-apikey: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0600 {
		t.Errorf("anthropic-apikey file mode = %o; want 0600", mode)
	}
}
