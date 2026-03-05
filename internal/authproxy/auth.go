package authproxy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const configDir = ".config/codex-dock"
const apiKeyFile = "apikey"

// AuthInfo describes the current auth configuration (no secrets).
type AuthInfo struct {
	Source        string
	KeyConfigured bool
}

// OAuthCredentials holds OAuth credentials loaded from ~/.codex/auth.json.
// Only AccessToken is passed to containers; RefreshToken stays on the host.
type OAuthCredentials struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"` // Unix timestamp; 0 means unknown
	TokenType    string `json:"token_type"`
}

// codexAuthFile represents the on-disk structure of ~/.codex/auth.json.
// Supports both the nested format (auth_mode + tokens object) used by
// ChatGPT/OAuth and the legacy flat format used by older Codex versions.
type codexAuthFile struct {
	AuthMode string `json:"auth_mode"`
	Tokens   *struct {
		IDToken      string `json:"id_token"`
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		AccountID    string `json:"account_id"`
	} `json:"tokens"`
	// Legacy flat fields
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"`
	TokenType    string `json:"token_type"`
}

// LoadOAuthCredentials reads OAuth credentials from ~/.codex/auth.json.
// Returns an error if the file does not exist, cannot be parsed, or contains
// no access_token.
// Supports both the nested format (auth_mode + tokens object) and the legacy
// flat format.
func LoadOAuthCredentials() (*OAuthCredentials, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("getting home directory: %w", err)
	}
	path := filepath.Join(home, ".codex", "auth.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading auth.json: %w", err)
	}
	var f codexAuthFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parsing auth.json: %w", err)
	}

	// Nested format: auth_mode + tokens object (ChatGPT/OAuth)
	if f.Tokens != nil && f.Tokens.AccessToken != "" {
		return &OAuthCredentials{
			AccessToken:  f.Tokens.AccessToken,
			RefreshToken: f.Tokens.RefreshToken,
			TokenType:    "Bearer",
		}, nil
	}

	// Legacy flat format
	if f.AccessToken == "" {
		return nil, fmt.Errorf("auth.json contains no access_token")
	}
	return &OAuthCredentials{
		AccessToken:  f.AccessToken,
		RefreshToken: f.RefreshToken,
		ExpiresAt:    f.ExpiresAt,
		TokenType:    f.TokenType,
	}, nil
}

// GetAuthInfo returns metadata about the current auth configuration.
func GetAuthInfo() (*AuthInfo, error) {
	info := &AuthInfo{}

	if os.Getenv("OPENAI_API_KEY") != "" {
		info.Source = "OPENAI_API_KEY env"
		info.KeyConfigured = true
		return info, nil
	}

	if key, err := readStoredKey(); err == nil && key != "" {
		info.Source = "codex-dock config"
		info.KeyConfigured = true
		return info, nil
	}

	// Check codex auth.json
	home, _ := os.UserHomeDir()
	authJSON := filepath.Join(home, ".codex", "auth.json")
	if _, err := os.Stat(authJSON); err == nil {
		if IsOAuthAuth() {
			info.Source = "~/.codex/auth.json (OAuth/ChatGPT subscription)"
		} else {
			info.Source = "~/.codex/auth.json"
		}
		info.KeyConfigured = true
		return info, nil
	}

	info.Source = "none"
	return info, nil
}

// IsOAuthAuth returns true when ~/.codex/auth.json contains ChatGPT subscription
// (OAuth) credentials rather than a plain API key. OAuth sessions are identified
// by auth_mode == "chatgpt", a nested tokens.refresh_token, or a top-level
// refresh_token (legacy flat format).
func IsOAuthAuth() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	data, err := os.ReadFile(filepath.Join(home, ".codex", "auth.json"))
	if err != nil {
		return false
	}
	var f codexAuthFile
	if err := json.Unmarshal(data, &f); err != nil {
		return false
	}
	if f.AuthMode == "chatgpt" {
		return true
	}
	if f.Tokens != nil && f.Tokens.RefreshToken != "" {
		return true
	}
	return f.RefreshToken != ""
}

// CodexAuthJSONPath returns the absolute path to ~/.codex/auth.json.
func CodexAuthJSONPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".codex", "auth.json"), nil
}

// SaveAPIKey persists an API key to the codex-dock config directory.
func SaveAPIKey(key string) error {
	dir, err := configDirPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	path := filepath.Join(dir, apiKeyFile)
	// Store as simple JSON for extensibility
	data, _ := json.Marshal(map[string]string{"key": key})
	return os.WriteFile(path, data, 0600)
}

// RotateTokens signals that all existing tokens should be considered invalid.
func RotateTokens() error {
	// Without a running proxy, we can only signal via a marker file.
	dir, err := configDirPath()
	if err != nil {
		return err
	}
	marker := filepath.Join(dir, ".rotate")
	return os.WriteFile(marker, []byte(fmt.Sprintf("%d", os.Getpid())), 0600)
}

// loadAPIKey returns the best available API key.
func loadAPIKey() string {
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		return key
	}
	if key, err := readStoredKey(); err == nil && key != "" {
		return key
	}
	if key := readCodexAuthJSON(); key != "" {
		return key
	}
	return ""
}

func readStoredKey() (string, error) {
	dir, err := configDirPath()
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, apiKeyFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		return "", err
	}
	return m["key"], nil
}

func readCodexAuthJSON() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	path := filepath.Join(home, ".codex", "auth.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return ""
	}
	// Common keys used by codex auth.json
	for _, k := range []string{"OPENAI_API_KEY", "api_key", "key", "token"} {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func configDirPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, configDir), nil
}
