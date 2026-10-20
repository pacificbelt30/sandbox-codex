package cmd

import (
	"slices"
	"testing"

	"github.com/pacificbelt30/codex-dock/internal/authproxy"
	"github.com/pacificbelt30/codex-dock/internal/networkpolicy"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func TestForwardPolicyConfigAndFlags(t *testing.T) {
	oldRules, oldAllow := proxyForwardRules, proxyForwardAllow
	t.Cleanup(func() { proxyForwardRules, proxyForwardAllow = oldRules, oldAllow; viper.Reset() })
	viper.Reset()
	viper.Set("proxy.forward_allow_domains", []string{"example.com"})
	viper.Set("proxy.forward_deny_domains", []string{"bad.example.com"})
	viper.Set("proxy.forward_allow_cidrs", []string{"203.0.113.0/24"})
	viper.Set("proxy.forward_deny_cidrs", []string{"203.0.113.9"})
	viper.Set("proxy.forward_allow_ports", []int{443})
	viper.Set("proxy.forward_deny_ports", []int{22})
	c := &cobra.Command{Use: "policy-test"}
	bindForwardPolicyFlags(c)
	if err := c.Flags().Set("forward-allow-port", "8443"); err != nil {
		t.Fatal(err)
	}
	applyForwardPolicyDefaults(c)
	rules := forwardPolicyRules()
	if len(rules.AllowDomains) != 1 || len(rules.DenyDomains) != 1 || len(rules.AllowCIDRs) != 1 || len(rules.DenyCIDRs) != 1 || len(rules.DenyPorts) != 1 || len(rules.AllowPorts) != 1 || rules.AllowPorts[0] != 8443 {
		t.Fatalf("config/flag priority lost: %+v", rules)
	}
	a := proxyRunArgs{name: "egress", role: authproxy.RoleEgress, blockPrivate: true, forwardPolicy: rules}
	args := buildProxyRunArgs(a)
	for _, pair := range [][2]string{{"--forward-allow-domain", "example.com"}, {"--forward-deny-domain", "bad.example.com"}, {"--forward-allow-cidr", "203.0.113.0/24"}, {"--forward-deny-cidr", "203.0.113.9"}, {"--forward-allow-port", "8443"}, {"--forward-deny-port", "22"}, {"--label", proxyPolicyLabel + "=" + forwardPolicyID(a)}} {
		if !containsSequence(args, pair[0], pair[1]) {
			t.Fatalf("missing %v", pair)
		}
	}
}

func TestModelPolicyFlagsOverrideHostConfigAndChangeProxyIdentity(t *testing.T) {
	old := proxyAllowedModels
	t.Cleanup(func() { proxyAllowedModels = old; viper.Reset() })
	viper.Reset()
	viper.Set("proxy.allowed_models", []string{"configured"})
	c := &cobra.Command{Use: "test-models"}
	c.Flags().StringArrayVar(&proxyAllowedModels, "allow-model", nil, "")
	applyForwardPolicyDefaults(c)
	if !slices.Equal(proxyAllowedModels, []string{"configured"}) {
		t.Fatal("host model policy ignored")
	}
	if err := c.Flags().Set("allow-model", "explicit"); err != nil {
		t.Fatal(err)
	}
	applyForwardPolicyDefaults(c)
	if !slices.Equal(proxyAllowedModels, []string{"explicit"}) {
		t.Fatal("explicit policy overridden")
	}
	a := proxyRunArgs{role: authproxy.RoleAuth, allowedModels: proxyAllowedModels}
	args := buildProxyRunArgs(a)
	if !containsSequence(args, "--allow-model", "explicit") {
		t.Fatal("policy not passed to proxy")
	}
	id := proxyTransportID(a)
	a.allowedModels = []string{"different"}
	if id == proxyTransportID(a) || id == "" {
		t.Fatal("changed model policy would silently reuse proxy")
	}
}

func TestOldOrChangedProxyPolicyRequiresExplicitRecreation(t *testing.T) {
	a := proxyRunArgs{name: "egress", role: authproxy.RoleEgress, blockPrivate: true, forwardPolicy: networkpolicy.Rules{DenyDomains: []string{"example.com"}}}
	id := forwardPolicyID(a)
	if err := checkProxyPolicy(id, a); err != nil {
		t.Fatal(err)
	}
	if err := checkProxyPolicy("", a); err == nil {
		t.Fatal("legacy proxy silently reused")
	}
	a.forwardPolicy.DenyDomains = []string{"other.example"}
	if err := checkProxyPolicy(id, a); err == nil {
		t.Fatal("changed policy silently ignored")
	}
	a.role = authproxy.RoleAuth
	if err := checkProxyPolicy("", a); err != nil {
		t.Fatal("egress rule affected auth role", err)
	}
}
