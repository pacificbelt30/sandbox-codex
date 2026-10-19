package workeregress

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/pacificbelt30/codex-dock/internal/networkpolicy"
)

// Rules narrows a host policy for one worker. A product cannot widen a host
// domain allowlist. The egress proxy still blocks private destinations.
func Rules(host networkpolicy.Rules, domains []string) (networkpolicy.Rules, error) {
	if len(domains) == 0 {
		return networkpolicy.Rules{}, fmt.Errorf("empty worker egress allowlist")
	}
	for _, domain := range domains {
		clean := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(domain)), ".")
		if clean == "" {
			return networkpolicy.Rules{}, fmt.Errorf("empty egress domain")
		}
		if len(host.AllowDomains) > 0 {
			within := false
			for _, allowed := range host.AllowDomains {
				allowed = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(allowed)), ".")
				if clean == allowed || strings.HasSuffix(clean, "."+allowed) {
					within = true
					break
				}
			}
			if !within {
				return networkpolicy.Rules{}, fmt.Errorf("worker domain %q exceeds host egress allowlist", domain)
			}
		}
	}
	host.AllowDomains = append([]string(nil), domains...)
	host.BlockPrivate = true
	if _, err := networkpolicy.Compile(host); err != nil {
		return networkpolicy.Rules{}, err
	}
	return host, nil
}

// ID uses the existing CLI policy identity format so both callers select the
// same immutable egress proxy for identical rules.
func ID(rules networkpolicy.Rules) string {
	for _, values := range []*[]string{&rules.AllowDomains, &rules.DenyDomains, &rules.AllowCIDRs, &rules.DenyCIDRs} {
		*values = append([]string(nil), *values...)
		slices.Sort(*values)
	}
	for _, values := range []*[]int{&rules.AllowPorts, &rules.DenyPorts} {
		*values = append([]int(nil), *values...)
		slices.Sort(*values)
	}
	encoded, _ := json.Marshal(struct {
		Version string
		Rules   networkpolicy.Rules
	}{"policy-v1", rules})
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func Name(rules networkpolicy.Rules) string { return "codex-http-policy-" + ID(rules)[:16] }
