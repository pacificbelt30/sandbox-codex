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
		info.Source = "~/.codex/auth.json"
		info.KeyConfigured = true
		return info, nil
	}

	info.Source = "none"
	return info, nil
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
