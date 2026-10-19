package cmd

import (
	"fmt"
	"net/url"

	"github.com/pacificbelt30/codex-dock/internal/authproxy"
	"github.com/pacificbelt30/codex-dock/internal/network"
	"github.com/pacificbelt30/codex-dock/internal/workeregress"
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
	rules, err := workeregress.Rules(forwardPolicyRules(), domains)
	if err != nil {
		return proxyRunArgs{}, err
	}
	spec := proxyRunArgs{upstreamProxy: proxyUpstreamURL, caFile: proxyCAFile, role: authproxy.RoleEgress, networkName: proxyRunNetwork, image: viper.GetString("proxy_image"), listenAddr: "0.0.0.0:18082", blockPrivate: true, forwardPolicy: rules}
	spec.name = workeregress.Name(rules)
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
