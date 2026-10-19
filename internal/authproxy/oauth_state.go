package authproxy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type oauthState struct {
	SourceHash  string          `json:"source_hash"`
	Credentials json.RawMessage `json:"credentials"`
}

// The source hash prevents an older refresh result from replacing a new login.
func oauthSourceHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (p *Proxy) restoreOAuthState(provider, sourceToken string, target any) error {
	if p.cfg.OAuthStateDir == "" {
		return nil
	}
	if err := secureStateDir(p.cfg.OAuthStateDir); err != nil {
		return err
	}
	path := filepath.Join(p.cfg.OAuthStateDir, provider+".json")
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("insecure OAuth state file %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var state oauthState
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("invalid OAuth state: %w", err)
	}
	if state.SourceHash != oauthSourceHash(sourceToken) {
		return nil
	}
	return json.Unmarshal(state.Credentials, target)
}

func secureStateDir(path string) error {
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("insecure OAuth state directory %s", path)
	}
	return nil
}

// persistOAuthState is called before publishing a refreshed credential in memory.
// Callers hold refreshMu. An atomic rename ensures restart sees either generation.
func (p *Proxy) persistOAuthState(provider string, credentials any) error {
	if p.cfg.OAuthStateDir == "" {
		return nil
	}
	if err := secureStateDir(p.cfg.OAuthStateDir); err != nil {
		return err
	}
	encoded, err := json.Marshal(credentials)
	if err != nil {
		return err
	}
	data, err := json.Marshal(oauthState{SourceHash: p.oauthSource[provider], Credentials: encoded})
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(p.cfg.OAuthStateDir, ".oauth-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if err := f.Chmod(0600); err != nil {
		_ = f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), filepath.Join(p.cfg.OAuthStateDir, provider+".json")); err != nil {
		return err
	}
	d, err := os.Open(p.cfg.OAuthStateDir)
	if err != nil {
		return err
	}
	defer func() { _ = d.Close() }()
	return d.Sync()
}
