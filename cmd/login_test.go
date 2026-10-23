package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pacificbelt30/codex-dock/pkg/cxd/credentialstore"
	"github.com/pelletier/go-toml/v2"
)

func TestLoginCreatesAndSelectsIndependentProfileWithoutHostCredentials(t *testing.T) {
	isolatedProfiles(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("OPENAI_API_KEY", "host-key-must-not-be-used")
	old := cfgFile
	cfgFile = filepath.Join(t.TempDir(), "config.toml")
	t.Cleanup(func() { cfgFile = old })
	original := "# keep network restrictions\n[proxy]\nforward_default_deny = true\nforward_allow_domains = ['=git.example']\n"
	if err := os.WriteFile(cfgFile, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	cmd := newLoginCommand()
	cmd.SetArgs([]string{"--agent", "claude", "--method", "api-key", "--profile", "work", "--key-stdin", "--skip-check"})
	cmd.SetIn(strings.NewReader("explicit-key\n"))
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	store, _ := openAuthProfile("work")
	value, err := store.Load(context.Background(), "work")
	if err != nil || value.AnthropicAPIKey != "explicit-key" || value.OpenAIAPIKey != "" || value.OpenAIOAuth != nil {
		t.Fatal("login did not isolate the explicitly entered provider credential")
	}
	data, err := os.ReadFile(cfgFile)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := toml.Unmarshal(data, &config); err != nil || config["auth_profile"] != "work" || !strings.Contains(string(data), original) {
		// The inserted admin-secret setting may split the original table, but
		// comments and policy values must remain byte-for-byte intact.
		if err != nil || config["auth_profile"] != "work" || !strings.Contains(string(data), "# keep network restrictions") || !strings.Contains(string(data), "forward_allow_domains = ['=git.example']") {
			t.Fatal("login changed unrelated global settings")
		}
	}
	proxy := config["proxy"].(map[string]any)
	if _, err := resolveProxyAdminSecret("", proxy["admin_secret_file"].(string)); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "explicit-key") || strings.Contains(output.String(), "host-key") || !strings.Contains(output.String(), "Connection check: skipped") {
		t.Fatal("login printed credentials or claimed an unperformed connection check")
	}
	info, _ := os.Stat(cfgFile)
	if info.Mode().Perm() != 0600 {
		t.Fatal("login configuration has unsafe permissions")
	}
}

func TestLoginDoesNotOverwriteAnExistingProfile(t *testing.T) {
	isolatedProfiles(t)
	store, _ := openAuthProfile("work")
	if err := store.Create(context.Background(), "work", credentialstore.Credentials{OpenAIAPIKey: "original"}); err != nil {
		t.Fatal(err)
	}
	cmd := newLoginCommand()
	cmd.SetArgs([]string{"--agent", "codex", "--method", "api-key", "--profile", "work", "--key-stdin", "--skip-check", "--no-select"})
	cmd.SetIn(strings.NewReader("replacement"))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err == nil {
		t.Fatal("login silently replaced an existing connection")
	}
	value, _ := store.Load(context.Background(), "work")
	if value.OpenAIAPIKey != "original" {
		t.Fatal("login changed an existing connection")
	}
}

func TestLoginConfigRejectsLinksAndConcurrentChanges(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.WriteFile(target, []byte("# original\n"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := readLoginConfig(link); err == nil {
		t.Fatal("login followed a configuration symlink")
	}
	if err := writeLoginConfig(context.Background(), target, []byte("outdated"), []byte("replacement")); err == nil {
		t.Fatal("login overwrote a concurrently changed configuration")
	}
	data, _ := os.ReadFile(target)
	if string(data) != "# original\n" {
		t.Fatal("failed login changed existing settings")
	}
}
