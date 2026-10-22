package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pacificbelt30/codex-dock/internal/authproxy"
	"github.com/pacificbelt30/codex-dock/internal/sandbox"
	"github.com/pacificbelt30/codex-dock/pkg/cxd/credentialstore"
	"github.com/spf13/viper"
)

func isolatedProfiles(t *testing.T) {
	t.Helper()
	previousDir, previousName := authProfileDir, authProfileName
	authProfileDir, authProfileName = filepath.Join(t.TempDir(), "profiles"), ""
	viper.Reset()
	t.Cleanup(func() { authProfileDir, authProfileName = previousDir, previousName; viper.Reset() })
}

func TestIndependentProfilesCannotBeSharedIntoWorker(t *testing.T) {
	isolatedProfiles(t)
	authProfileName = "selected"
	root, err := authProfilesDirectory()
	if err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	if err := ensureProfilesOutsideWorkerShares(project, "", nil); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{root, filepath.Dir(root), filepath.Join(root, "selected")} {
		if err := ensureProfilesOutsideWorkerShares(path, "", nil); err == nil {
			t.Fatal("profile storage exposed through workspace")
		}
		if err := ensureProfilesOutsideWorkerShares(project, "", []sandbox.MountSpec{{Source: path, Target: "/shared", ReadOnly: true}}); err == nil {
			t.Fatal("profile storage exposed through readonly bind")
		}
	}
	alias := filepath.Join(project, "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	if err := ensureProfilesOutsideWorkerShares(alias, "", nil); err == nil {
		t.Fatal("symlink alias exposed profiles")
	}
	if err := ensureProfilesOutsideWorkerShares(project, filepath.Join(alias, "not-created-yet"), nil); err == nil {
		t.Fatal("missing clone destination beneath symlink exposed profiles")
	}
	old := runProxyAdminSecretFile
	t.Cleanup(func() { runProxyAdminSecretFile = old })
	runProxyAdminSecretFile = filepath.Join(project, "admin-secret")
	if err := ensureProfilesOutsideWorkerShares(project, "", nil); err == nil {
		t.Fatal("admin authority exposed through workspace")
	}
}

func TestIndependentInitUsesOnlySelectedProfile(t *testing.T) {
	isolatedProfiles(t)
	t.Setenv("OPENAI_API_KEY", "fixture-host-key")
	t.Setenv("ANTHROPIC_API_KEY", "fixture-host-key")
	authProfileName = "selected"
	if err := checkInitCredentials(context.Background(), "codex"); err == nil {
		t.Fatal("init used host credentials for a missing profile")
	}
	store, err := openAuthProfile("selected")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(context.Background(), "selected", credentialstore.Credentials{OpenAIAPIKey: "fixture-profile"}); err != nil {
		t.Fatal(err)
	}
	if err := checkInitCredentials(context.Background(), "codex"); err != nil {
		t.Fatal(err)
	}
	if err := checkInitCredentials(context.Background(), "claude"); err == nil {
		t.Fatal("init used host Anthropic credentials")
	}
}

func TestIndependentProxyConfigurationMountsOnlySelectedProfile(t *testing.T) {
	isolatedProfiles(t)
	store, err := openAuthProfile("selected")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(context.Background(), "selected", credentialstore.Credentials{OpenAIAPIKey: "fixture-profile-key"}); err != nil {
		t.Fatal(err)
	}
	v, _ := store.Load(context.Background(), "selected")
	a := proxyRunArgs{profileName: "selected", profileDirectory: store.Directory(), profileConnection: v.ConnectionVersion,
		role: authproxy.RoleAuth, image: "fixture:latest", name: "proxy", listenAddr: "0.0.0.0:18080"}
	c, h, err := proxyConfiguration(a)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Env) != 0 || c.User != fmt.Sprintf("%d:%d", os.Geteuid(), os.Getegid()) || len(h.Mounts) != 1 || !hasProfileMount(h, store.Directory()) || c.Labels[proxyProfileLabel] != profileProxyID(a) {
		t.Fatal("profile proxy gained host auth or lost its exclusive writable directory")
	}
	if !containsSequence(c.Cmd, "--auth-profile", "selected") || !containsSequence(c.Cmd, "--profile-store-dir", "/run/cxd-profile") || strings.Contains(strings.Join(c.Cmd, " "), "fixture-profile-key") {
		t.Fatal("profile selection missing or credential exposed in argv")
	}
	for _, bad := range []proxyRunArgs{
		func() proxyRunArgs { b := a; b.role = authproxy.RoleEgress; return b }(),
		func() proxyRunArgs { b := a; b.mountCreds = true; return b }(),
		func() proxyRunArgs { b := a; b.profileConnection = "stale"; return b }(),
	} {
		if _, _, err := proxyConfiguration(bad); err == nil {
			t.Fatal("unsafe or stale profile mount accepted")
		}
	}
}

func TestIndependentRunReconciliationDoesNotUseHostCredentials(t *testing.T) {
	isolatedProfiles(t)
	authProfileName = "selected"
	t.Setenv("OPENAI_API_KEY", "host-key")
	t.Setenv("ANTHROPIC_API_KEY", "host-anthropic")
	store, err := openAuthProfile("selected")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(context.Background(), "selected", credentialstore.Credentials{OpenAIAPIKey: "fixture-profile"}); err != nil {
		t.Fatal(err)
	}
	v, _ := store.Load(context.Background(), "selected")
	for _, profile := range []string{"selected", "other", ""} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Cxd-Auth-Profile", profile)
			w.Header().Set("X-Cxd-Connection-Version", v.ConnectionVersion)
			_ = json.NewEncoder(w).Encode(map[string]bool{"worker_token_auth": true, "independent_profile": profile != ""})
		}))
		remote, err := authproxy.NewRemoteProxy(server.URL, "http://fixture-proxy", "")
		if err != nil {
			t.Fatal(err)
		}
		_, err = reconcileProxyAuthState(context.Background(), remote, true)
		if (err == nil) != (profile == "selected") {
			t.Fatal("run selected host auth, another profile or automatic forced recreation")
		}
		if profile == "selected" {
			authProfileName = ""
			if _, err := reconcileProxyAuthState(context.Background(), remote, false); err == nil {
				t.Fatal("profile proxy accepted without explicit profile selection")
			}
			authProfileName = "selected"
		}
		server.Close()
	}
}

func profileCommand(args []string, input string) (string, error) {
	command := newAuthProfileCommand()
	command.SilenceUsage, command.SilenceErrors = true, true
	command.SetArgs(args)
	command.SetIn(strings.NewReader(input))
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	err := command.Execute()
	return output.String(), err
}

func TestIndependentProfileCLIInputAndRedactedOutput(t *testing.T) {
	isolatedProfiles(t)
	t.Setenv("OPENAI_API_KEY", "fixture-host-key-must-not-be-used")
	if _, err := profileCommand([]string{"set", "host"}, ""); err == nil {
		t.Fatal("host key was implicitly imported without explicit input")
	}
	for _, provider := range []string{"openai", "anthropic"} {
		secret := "fixture-product-" + provider
		output, err := profileCommand([]string{"set", provider, "--provider", provider, "--key-stdin"}, secret+"\n")
		if err != nil || strings.Contains(output, secret) {
			t.Fatal("profile creation failed or printed its credential")
		}
		output, err = profileCommand([]string{"show", provider}, "")
		if err != nil || !strings.Contains(output, "api-key") || strings.Contains(output, secret) {
			t.Fatal("profile metadata failed or leaked a credential")
		}
		if _, err := profileCommand([]string{"set", provider, "--key-stdin"}, "replacement"); err == nil {
			t.Fatal("profile silently overwritten")
		}
		store, err := openAuthProfile(provider)
		if err != nil {
			t.Fatal(err)
		}
		value, err := store.Load(context.Background(), provider)
		if err != nil || value.OpenAIAPIKey != secret && value.AnthropicAPIKey != secret {
			t.Fatal("profile changed or used host authentication")
		}
	}
	if _, err := profileCommand([]string{"show", "missing"}, ""); err == nil {
		t.Fatal("missing profile fell back to host credentials")
	}
	output, err := profileCommand([]string{"list"}, "")
	if err != nil || !strings.Contains(output, "openai") || !strings.Contains(output, "anthropic") || strings.Contains(output, "fixture-") {
		t.Fatal("profile listing failed or leaked credentials")
	}
}

func TestIndependentProfileImportsNativeOAuthExplicitly(t *testing.T) {
	isolatedProfiles(t)
	for _, tc := range []struct{ name, format, input string }{
		{"codex", "codex", `{"auth_mode":"chatgpt","tokens":{"id_token":"fixture-id","access_token":"fixture-access","refresh_token":"fixture-refresh","account_id":"fixture-account"},"last_refresh":"2026-10-22T00:00:00Z"}`},
		{"claude", "claude", `{"claudeAiOauth":{"accessToken":"fixture-access","refreshToken":"fixture-refresh","expiresAt":123456,"scopes":["user:inference"]}}`},
		{"both", "credentials", `{"openai_api_key":"fixture-openai","anthropic_api_key":"fixture-anthropic"}`},
	} {
		if _, err := profileCommand([]string{"import", tc.name, "--format", tc.format}, tc.input); err == nil {
			t.Fatal("import did not require explicit stdin opt-in")
		}
		output, err := profileCommand([]string{"import", tc.name, "--format", tc.format, "--credentials-stdin"}, tc.input)
		if err != nil || strings.Contains(output, "fixture-") {
			t.Fatal("explicit native import failed or leaked credentials")
		}
	}
	for _, input := range []string{`null`, `{"openai_api_key":"fixture-secret",`, `{"openai_api_key":"fixture-key","openai_oauth":{}}`} {
		output, err := profileCommand([]string{"import", "invalid", "--credentials-stdin"}, input)
		if err == nil || strings.Contains(output+err.Error(), "fixture-") {
			t.Fatal("invalid credential input accepted or echoed")
		}
	}
}
