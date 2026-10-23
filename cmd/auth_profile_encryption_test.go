package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/pacificbelt30/codex-dock/internal/authproxy"
	"github.com/pacificbelt30/codex-dock/internal/diagnostics"
	"github.com/pacificbelt30/codex-dock/pkg/cxd/credentialstore"
	"github.com/spf13/cobra"
)

func encryptedCommandFixture(t *testing.T) (*credentialstore.EncryptedStore, []byte) {
	t.Helper()
	isolatedProfiles(t)
	plain, err := openAuthProfile("work")
	if err != nil {
		t.Fatal(err)
	}
	store, err := credentialstore.NewEncryptedStore(plain.Directory(), "work")
	if err != nil {
		t.Fatal(err)
	}
	if err = store.CreateWithPassphrase(context.Background(), "work", []byte("correct horse battery staple"), credentialstore.Credentials{OpenAIAPIKey: "fixture-encrypted-command-key"}); err != nil {
		t.Fatal(err)
	}
	key, err := store.UnlockedKey()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { clear(key); _ = store.Close() })
	return store, key
}
func TestEncryptedProfileMetadataDoesNotPromptOrDisclose(t *testing.T) {
	_, _ = encryptedCommandFixture(t)
	authProfileName = "work"
	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.SetOut(&out)
	if err := showAuthProfile(cmd, "work"); err != nil {
		t.Fatal(err)
	}
	report := diagnostics.Report{}
	profileStatus(context.Background(), &report, false)
	data, _ := json.Marshal(report)
	if report.Err() != nil || !strings.Contains(out.String(), "locked") || !strings.Contains(string(data), "not decrypted") || strings.Contains(out.String()+string(data), "fixture-encrypted-command-key") {
		t.Fatal("metadata required unlock or leaked credentials")
	}
}
func TestEncryptedProxyKeyOnlyUsesStdin(t *testing.T) {
	store, key := encryptedCommandFixture(t)
	info, _ := store.Info(context.Background())
	v, _ := store.Load(context.Background(), "work")
	spec := proxyRunArgs{image: "proxy:test", role: authproxy.RoleAuth, profileName: "work", profileDirectory: store.Directory(), profileConnection: v.ConnectionVersion, profileFingerprint: info.Fingerprint, profileKey: key}
	cfg, host, err := proxyConfiguration(spec)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal([]interface{}{cfg, host})
	args := fmt.Sprint(buildProxyRunArgs(spec))
	for _, arg := range cfg.Cmd[2:] {
		if strings.HasPrefix(arg, "--") {
			name, _, _ := strings.Cut(strings.TrimPrefix(arg, "--"), "=")
			if proxyServeCmd.Flags().Lookup(name) == nil && rootCmd.PersistentFlags().Lookup(name) == nil {
				t.Fatalf("generated proxy server option is unknown: %s", name)
			}
		}
	}
	if !cfg.OpenStdin || !strings.Contains(args, "--profile-key-stdin") || bytes.Contains(data, key) || strings.Contains(string(data)+args, "fixture-encrypted-command-key") {
		t.Fatal("key or credentials leaked into metadata, or stdin missing")
	}
	spec.profileKey = bytes.Repeat([]byte{1}, 32)
	if _, _, err = proxyConfiguration(spec); err == nil {
		t.Fatal("incorrect data key accepted")
	}
	spec.profileKey = key
	spec.profileConnection = "old"
	if _, _, err = proxyConfiguration(spec); err == nil {
		t.Fatal("stale connection accepted")
	}
}
func TestPassphraseDescriptorBoundedAndNotFromEnvironment(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte("descriptor passphrase\n"))
	_ = w.Close()
	pass, err := readPassphraseFD(int(r.Fd()))
	if err != nil || string(pass) != "descriptor passphrase" {
		t.Fatal("explicit FD failed", err)
	}
	clear(pass)
	if _, err = readPassphraseFD(0); err == nil {
		t.Fatal("stdin silently used as passphrase input")
	}
}

func TestProfileLockScopesSameNamesByProxyStore(t *testing.T) {
	scopes := map[string]string{"proxy-a": "/private/a/work", "proxy-b": "/private/b/work"}
	for _, tc := range []struct {
		endpoint        string
		selected, known bool
	}{
		{"http://proxy-a:18080", true, true}, {"http://proxy-b:18080", false, true}, {"http://removed:18080", false, false}, {"", false, false},
	} {
		selected, known := workerProfileScope(map[string]string{"codex-dock.auth-profile": "work", "codex-dock.proxy-container-url": tc.endpoint}, scopes, "/private/a/work")
		if selected != tc.selected || known != tc.known {
			t.Fatal("profile lock crossed storage roots or accepted an unknown worker scope")
		}
	}
}
