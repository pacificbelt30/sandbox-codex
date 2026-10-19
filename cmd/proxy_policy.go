package cmd

import (
	"fmt"
	"github.com/pacificbelt30/codex-dock/internal/networkpolicy"
	"github.com/pacificbelt30/codex-dock/internal/workeregress"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var proxyForwardRules networkpolicy.Rules

const proxyPolicyLabel = "io.codex-dock.forward-policy"

func effectiveForwardRules(a proxyRunArgs) networkpolicy.Rules {
	rules := a.forwardPolicy
	if len(a.forwardAllow) != 0 {
		rules.AllowDomains = a.forwardAllow
	}
	rules.BlockPrivate = rules.BlockPrivate || a.blockPrivate
	return rules
}

func forwardPolicyID(a proxyRunArgs) string {
	return workeregress.ID(effectiveForwardRules(a))
}

func checkProxyPolicy(actual string, a proxyRunArgs) error {
	if a.role == "egress" && actual != forwardPolicyID(a) {
		return fmt.Errorf("egress proxy %q has an old or different network policy; drain workers, rebuild the proxy image and recreate the egress proxy before starting new workers (Kata workers pin proxy IPs)", a.name)
	}
	return nil
}

func bindForwardPolicyFlags(cmd *cobra.Command) {
	flags := cmd.Flags()
	flags.StringArrayVar(&proxyForwardAllow, "forward-allow-domain", nil, "Allow these domains and subdomains (repeatable; empty allows all)")
	flags.StringArrayVar(&proxyForwardRules.DenyDomains, "forward-deny-domain", nil, "Deny these domains and subdomains (repeatable; deny overrides allow)")
	flags.StringArrayVar(&proxyForwardRules.AllowCIDRs, "forward-allow-cidr", nil, "Allow only these destination IPs/CIDRs (repeatable)")
	flags.StringArrayVar(&proxyForwardRules.DenyCIDRs, "forward-deny-cidr", nil, "Deny these destination IPs/CIDRs (repeatable)")
	flags.IntSliceVar(&proxyForwardRules.AllowPorts, "forward-allow-port", nil, "Allow only these destination ports (comma-separated or repeatable)")
	flags.IntSliceVar(&proxyForwardRules.DenyPorts, "forward-deny-port", nil, "Deny these destination ports (comma-separated or repeatable)")
}

func applyForwardPolicyDefaults(cmd *cobra.Command) {
	for _, setting := range []struct {
		flag, key string
		value     *[]string
	}{
		{"forward-allow-domain", "proxy.forward_allow_domains", &proxyForwardAllow},
		{"forward-deny-domain", "proxy.forward_deny_domains", &proxyForwardRules.DenyDomains},
		{"forward-allow-cidr", "proxy.forward_allow_cidrs", &proxyForwardRules.AllowCIDRs},
		{"forward-deny-cidr", "proxy.forward_deny_cidrs", &proxyForwardRules.DenyCIDRs},
	} {
		if !cmd.Flags().Changed(setting.flag) && viper.IsSet(setting.key) {
			*setting.value = viper.GetStringSlice(setting.key)
		}
	}
	for _, setting := range []struct {
		flag, key string
		value     *[]int
	}{
		{"forward-allow-port", "proxy.forward_allow_ports", &proxyForwardRules.AllowPorts},
		{"forward-deny-port", "proxy.forward_deny_ports", &proxyForwardRules.DenyPorts},
	} {
		if !cmd.Flags().Changed(setting.flag) && viper.IsSet(setting.key) {
			*setting.value = viper.GetIntSlice(setting.key)
		}
	}
}

func forwardPolicyRules() networkpolicy.Rules {
	rules := proxyForwardRules
	rules.AllowDomains = proxyForwardAllow
	return rules
}
