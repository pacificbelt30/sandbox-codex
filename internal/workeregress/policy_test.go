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
