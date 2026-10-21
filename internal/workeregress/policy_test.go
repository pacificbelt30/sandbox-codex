package workeregress

import (
	"testing"

	"github.com/pacificbelt30/codex-dock/internal/networkpolicy"
)

func TestRulesCannotWidenHostPolicy(t *testing.T) {
	host := networkpolicy.Rules{AllowDomains: []string{"example.com"}, DenyDomains: []string{"blocked.example.com"}, AllowPorts: []int{443}}
	rules, err := Rules(host, []string{"api.example.com"})
	if err != nil || !rules.BlockPrivate || len(rules.AllowDomains) != 1 || rules.AllowDomains[0] != "api.example.com" || len(rules.DenyDomains) != 1 || len(rules.AllowPorts) != 1 {
		t.Fatalf("narrowed rules = %+v, %v", rules, err)
	}
	if _, err := Rules(host, []string{"outside.example.org"}); err == nil {
		t.Fatal("host policy was widened")
	}
	if _, err := Rules(host, []string{"bad domain"}); err == nil {
		t.Fatal("invalid domain was accepted")
	}
}

func TestPolicyIDIsOrderIndependent(t *testing.T) {
	a, err := Rules(networkpolicy.Rules{}, []string{"a.example.com", "b.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := Rules(networkpolicy.Rules{}, []string{"b.example.com", "a.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if ID(a) != ID(b) || Name(a) != Name(b) {
		t.Fatal("equivalent policies selected different proxies")
	}
}

func TestStrictHostPolicyCannotBeWidened(t *testing.T) {
	if _, err := Rules(networkpolicy.Rules{DefaultDeny: true}, []string{"=example.com"}); err == nil {
		t.Fatal("worker widened an empty host grant")
	}
	host := networkpolicy.Rules{DefaultDeny: true, AllowDomains: []string{"=api.example.com"}}
	if _, err := Rules(host, []string{"api.example.com"}); err == nil {
		t.Fatal("worker added subdomains to an exact host rule")
	}
	if _, err := Rules(host, []string{"=api.example.com."}); err != nil {
		t.Fatal("equivalent exact host was rejected:", err)
	}
	if _, offline, err := ResolvePreset(networkpolicy.Rules{DefaultDeny: true}, "", nil); err != nil || !offline {
		t.Fatalf("empty strict policy did not select offline: %t %v", offline, err)
	}
}

func TestLegacyPolicyIdentityDoesNotChangeOnUpgrade(t *testing.T) {
	rules := networkpolicy.Rules{AllowDomains: []string{"example.com"}, BlockPrivate: true}
	const previous = "eb75769a902660894af7cc31dfabfbceca930c8421e9645c4e6444c30be83a25"
	if ID(rules) != previous {
		t.Fatal("omitted new settings changed the identity of an existing proxy")
	}
	rules.DefaultDeny = true
	if ID(rules) == previous {
		t.Fatal("strict policy reused the legacy proxy identity")
	}
	rules.DefaultDeny = false
	rules.AllowDomains = []string{"=example.com"}
	if ID(rules) == previous {
		t.Fatal("exact policy reused a subdomain policy identity")
	}
}
