package cmd

import (
	"fmt"
	"github.com/pacificbelt30/codex-dock/internal/authproxy"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestNativeProxyConfigurationSeparatesCredentialsAndPorts(t *testing.T) {
	a := proxyRunArgs{name: "auth", image: "proxy:dev", role: authproxy.RoleAuth, networkName: "egress", mountCreds: true, apiKeyEnv: "fixture", listenAddr: "0.0.0.0:18080", adminPort: 18081}
	c, h, err := proxyConfiguration(a)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(c.Env, "OPENAI_API_KEY=fixture") || !slices.Contains(c.Env, "HOME="+proxyCredentialHome) || len(h.PortBindings) != 1 || h.Runtime != "runc" {
		t.Fatalf("auth configuration differs: %v %v", c, h)
	}
	a.role = authproxy.RoleEgress
	a.mountCreds = false
	a.adminPort = 0
	c, h, err = proxyConfiguration(a)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Env) != 0 || len(h.Mounts) != 0 || len(h.PortBindings) != 0 || c.Labels[proxyPolicyLabel] == "" {
		t.Fatal("egress gained credentials/ports or lost policy identity")
	}
}

func TestNativeProxyReadsOwnerOnlyCredentialMounts(t *testing.T) {
	dir := t.TempDir()
	paths := make([]string, 4)
	for i := range paths {
		paths[i] = filepath.Join(dir, fmt.Sprint(i))
		if err := os.WriteFile(paths[i], []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	c, h, err := proxyConfiguration(proxyRunArgs{image: "proxy:dev", role: authproxy.RoleAuth, mountCreds: true, storedKeyPath: paths[0], oauthJSONPath: paths[1], anthropicKeyPath: paths[2], claudeCredsPath: paths[3]})
	if err != nil {
		t.Fatal(err)
	}
	if c.User != fmt.Sprintf("%d:%d", os.Geteuid(), os.Getegid()) || !slices.Contains(c.Env, "HOME="+proxyCredentialHome) || len(h.Mounts) != 4 {
		t.Fatal("proxy did not retain credential owner and home")
	}
	if !slices.Equal([]string(h.CapDrop), []string{"ALL"}) || !slices.Contains(h.SecurityOpt, "no-new-privileges:true") {
		t.Fatal("proxy privileges were widened")
	}
	for _, m := range h.Mounts {
		if !m.ReadOnly || !strings.HasPrefix(m.Target, proxyCredentialHome+"/") {
			t.Fatal("credential mount is writable or outside the proxy home")
		}
		info, err := os.Stat(m.Source)
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatal("source credential permissions changed")
		}
	}
}

func TestProxyCredentialAccessRejectsUnreadableIdentity(t *testing.T) {
	p := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(p, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := checkProxyCredentialAccess(p, os.Geteuid()+1, os.Getegid()+1); err == nil {
		t.Fatal("unreadable owner-only credential accepted")
	}
	if err := checkProxyCredentialAccess(filepath.Dir(p), os.Geteuid(), os.Getegid()); err == nil {
		t.Fatal("directory accepted as a credential")
	}
}

func TestNativeProxyMountsOnlyOAuthStateWritable(t *testing.T) {
	state := filepath.Join(t.TempDir(), "oauth")
	if err := os.Mkdir(state, 0700); err != nil {
		t.Fatal(err)
	}
	c, h, err := proxyConfiguration(proxyRunArgs{image: "proxy:dev", role: authproxy.RoleAuth, mountCreds: true, oauthStateDir: state})
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Mounts) != 1 || h.Mounts[0].ReadOnly || h.Mounts[0].Source != state || !slices.Contains(c.Cmd, "--oauth-state-dir") {
		t.Fatal("OAuth state mount or CLI path missing")
	}
}
