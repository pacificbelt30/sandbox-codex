package cmd

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/pacificbelt30/codex-dock/internal/authproxy"
	"github.com/pacificbelt30/codex-dock/internal/network"
	"github.com/pacificbelt30/codex-dock/internal/networkpolicy"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// workerEgressSpec creates an immutable policy-specific egress proxy. A worker
// network connects to exactly this instance rather than the unrestricted one.
func workerEgressSpec(cmd *cobra.Command, domains []string) (proxyRunArgs, error) {
	if len(domains) == 0 {
		return proxyRunArgs{}, fmt.Errorf("empty worker egress allowlist")
	}
	applyForwardPolicyDefaults(cmd)
	rules := forwardPolicyRules()
	for _, domain := range domains {
		clean := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(domain)), ".")
		if clean == "" {
			return proxyRunArgs{}, fmt.Errorf("empty egress domain")
		}
		if len(rules.AllowDomains) > 0 {
			withinHostPolicy := false
			for _, hostDomain := range rules.AllowDomains {
				hostDomain = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(hostDomain)), ".")
				if clean == hostDomain || strings.HasSuffix(clean, "."+hostDomain) {
					withinHostPolicy = true
					break
				}
			}
			if !withinHostPolicy {
				return proxyRunArgs{}, fmt.Errorf("worker domain %q exceeds host egress allowlist", domain)
			}
		}
	}
	rules.AllowDomains = append([]string(nil), domains...)
	rules.BlockPrivate = true
	if _, err := networkpolicy.Compile(rules); err != nil {
		return proxyRunArgs{}, err
	}
	spec := proxyRunArgs{role: authproxy.RoleEgress, networkName: proxyRunNetwork, image: viper.GetString("proxy_image"), listenAddr: "0.0.0.0:18082", blockPrivate: true, forwardPolicy: rules}
	spec.name = "codex-http-policy-" + forwardPolicyID(spec)[:16]
	return spec, nil
}

func ensureWorkerEgressProxy(cmd *cobra.Command, domains []string) (string, error) {
	ctx := cmd.Context()
	spec, err := workerEgressSpec(cmd, domains)
	if err != nil {
		return "", err
	}
	if err := ensureBridgeNetwork(ctx, spec.networkName, network.ProxyBridgeName); err != nil {
		return "", err
	}
	if err := ensureProxyImage(ctx, spec.image); err != nil {
		return "", err
	}
	if err := ensureProxyContainer(ctx, spec); err != nil {
		return "", err
	}
	return (&url.URL{Scheme: "http", Host: fmt.Sprintf("%s:18082", spec.name)}).String(), nil
}
