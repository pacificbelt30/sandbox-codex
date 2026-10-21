package workeregress

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/pacificbelt30/codex-dock/internal/networkpolicy"
)

// Rules narrows a host policy for one worker. A product cannot widen a host
// domain allowlist. The egress proxy still blocks private destinations.
func Rules(host networkpolicy.Rules, domains []string) (networkpolicy.Rules, error) {
	if len(domains) == 0 {
		return networkpolicy.Rules{}, fmt.Errorf("empty worker egress allowlist")
	}
	if _, err := networkpolicy.Compile(host); err != nil {
		return networkpolicy.Rules{}, err
	}
	if host.DefaultDeny && len(host.AllowDomains) == 0 && len(host.AllowCIDRs) == 0 {
		return networkpolicy.Rules{}, fmt.Errorf("host default-deny policy grants no destinations")
	}
	for _, domain := range domains {
		if len(host.AllowDomains) > 0 {
			within := false
			for _, allowed := range host.AllowDomains {
				matches, err := networkpolicy.DomainWithin(domain, allowed)
				if err != nil {
					return networkpolicy.Rules{}, err
				}
				if matches {
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

// Configured reports restrictions beyond mandatory private-address blocking.
func Configured(r networkpolicy.Rules) bool {
	return r.DefaultDeny || len(r.AllowDomains)+len(r.DenyDomains)+len(r.AllowCIDRs)+len(r.DenyCIDRs)+len(r.AllowPorts)+len(r.DenyPorts) > 0
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
