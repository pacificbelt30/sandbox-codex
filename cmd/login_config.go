package cmd

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"github.com/pacificbelt30/codex-dock/internal/onboarding"
	"github.com/pacificbelt30/codex-dock/pkg/cxd/credentialstore"
	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/viper"
)

func selectLoginProfile(ctx context.Context, output io.Writer, profile string) error {
	if err := credentialstore.ValidateProfileName(profile); err != nil {
		return err
	}
	configDir, err := defaultConfigDir()
	if err != nil {
		return err
	}
	path := filepath.Join(configDir, "config.toml")
	if cfgFile != "" {
		path, err = filepath.Abs(cfgFile)
		if err != nil {
			return err
		}
	}
	data, err := readLoginConfig(path)
	if err != nil {
		return err
	}
	var parsed map[string]any
	if err := toml.Unmarshal(data, &parsed); err != nil {
		return fmt.Errorf("invalid existing global configuration")
	}
	secretPath := filepath.Join(configDir, "proxy-admin-secret")
	proxy, _ := parsed["proxy"].(map[string]any)
	if value, ok := proxy["admin_secret"].(string); ok && value != "" {
		if len(value) < 32 {
			return fmt.Errorf("the existing proxy admin secret is too short for independent profiles; configure an owner-only admin-secret file")
		}
	} else if value, ok := proxy["admin_secret_file"].(string); ok && value != "" {
		if _, err := resolveProxyAdminSecret("", value); err != nil {
			return err
		}
	} else {
		if err := prepareLoginAdminSecret(secretPath); err != nil {
			return err
		}
	}
	updated, err := onboarding.SelectProfile(data, profile, secretPath)
	if err != nil {
		return err
	}
	if err := writeLoginConfig(ctx, path, data, updated); err != nil {
		return err
	}
	viper.Set("auth_profile", profile)
	if proxy["admin_secret"] == nil && proxy["admin_secret_file"] == nil {
		viper.Set("proxy.admin_secret_file", secretPath)
	}
	_, err = fmt.Fprintf(output, "Selected profile: %s\nSettings: %s\n", profile, path)
	return err
}

func prepareLoginAdminSecret(path string) error {
	if _, err := os.Lstat(path); err == nil {
		_, err = resolveProxyAdminSecret("", path)
		return err
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	var value [32]byte
	if _, err := rand.Read(value[:]); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0600)
	if os.IsExist(err) {
		_, err = resolveProxyAdminSecret("", path)
		return err
	}
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	if _, err := io.WriteString(f, hex.EncodeToString(value[:])+"\n"); err != nil {
		return err
	}
	return f.Sync()
}

func readLoginConfig(path string) ([]byte, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("opening global login settings failed")
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	if !ok || owner.Uid != uint32(os.Geteuid()) || owner.Nlink != 1 || !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 {
		return nil, fmt.Errorf("global login settings must be an owned regular file without group/other write permission")
	}
	data, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return nil, fmt.Errorf("global login settings exceed the supported size")
	}
	return data, nil
}

func writeLoginConfig(ctx context.Context, path string, expected, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	current, err := readLoginConfig(path)
	if err != nil {
		return err
	}
	if !bytes.Equal(current, expected) {
		return fmt.Errorf("global settings changed while preparing login; select the saved profile again")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".oaiso-login-*")
	if err != nil {
		return err
	}
	defer func() { _ = f.Close(); _ = os.Remove(f.Name()) }()
	if err := f.Chmod(0600); err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer func() { _ = dir.Close() }()
	return dir.Sync()
}
