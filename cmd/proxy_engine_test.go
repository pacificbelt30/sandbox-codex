package cmd

import (
	"github.com/pacificbelt30/codex-dock/internal/authproxy"
	"testing"
)

func TestNativeProxyConfigurationSeparatesCredentialsAndPorts(t *testing.T) {
	a := proxyRunArgs{name: "auth", image: "proxy:dev", role: authproxy.RoleAuth, networkName: "egress", mountCreds: true, apiKeyEnv: "fixture", listenAddr: "0.0.0.0:18080", adminPort: 18081}
	c, h, err := proxyConfiguration(a)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Env) != 1 || c.Env[0] != "OPENAI_API_KEY=fixture" || len(h.PortBindings) != 1 || h.Runtime != "runc" {
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
