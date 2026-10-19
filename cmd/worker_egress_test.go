package cmd

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestWorkerEgressProxyUsesPolicySpecificIdentity(t *testing.T) {
	oldRules, oldAllow := proxyForwardRules, proxyForwardAllow
	defer func() { proxyForwardRules, proxyForwardAllow = oldRules, oldAllow }()
	proxyForwardRules.AllowDomains = nil
	proxyForwardAllow = []string{"example.com"}
	viper.Set("proxy_image", "proxy:test")
	defer viper.Set("proxy_image", nil)
	one, err := workerEgressSpec(runCmd, []string{"api.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	two, err := workerEgressSpec(runCmd, []string{"other.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if one.name == two.name || !strings.HasPrefix(one.name, "codex-http-policy-") {
		t.Fatal("workers with different rules share a proxy")
	}
	if one.mountCreds || one.adminPort != 0 {
		t.Fatal("policy proxy gained credentials or admin")
	}
	if _, err := workerEgressSpec(runCmd, []string{"outside.example.org"}); err == nil {
		t.Fatal("worker widened host egress policy")
	}
	if _, err := workerEgressSpec(runCmd, []string{"bad domain"}); err == nil {
		t.Fatal("invalid domain accepted")
	}
}
