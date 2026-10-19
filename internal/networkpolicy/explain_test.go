package networkpolicy

import (
	"context"
	"errors"
	"net/netip"
	"strings"
	"testing"
)

type explainResolver []netip.Addr

func (r explainResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return r, nil
}
func TestExplainStages(t *testing.T) {
	rules := Rules{AllowDomains: []string{"example.com"}, BlockPrivate: true}
	d, err := Explain(context.Background(), rules, "api.example.com", false, nil)
	if err != nil || !d.Allowed || d.Complete || d.Code != "address_not_checked" {
		t.Fatalf("%+v %v", d, err)
	}
	d, err = Explain(context.Background(), rules, "api.example.com", true, explainResolver{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("127.0.0.1")})
	if err != nil || d.Allowed || !d.Complete || d.Code != "private_address" || d.AddressesChecked != 2 {
		t.Fatalf("%+v %v", d, err)
	}
	p, _ := Compile(rules)
	err = p.CheckDestination(context.Background(), Destination{"other.com", 443})
	if !errors.Is(err, ErrDenied) || Reason(err) != "domain_not_allowed" {
		t.Fatal(err)
	}
}
func TestDomainIndexEquivalence(t *testing.T) {
	domains := []string{"example.com", "a.example.org", "8.8.8.8", "2001:4860:4860::8888", "localhost", "com"}
	idx := indexDomains(domains)
	for _, base := range append(append([]string{}, domains...), "notexample.net", "other.org") {
		for _, prefix := range []string{"", "a.", "a.b.", "evil"} {
			host := prefix + base
			want := false
			for _, domain := range domains {
				if host == domain {
					want = true
				}
				if _, err := netip.ParseAddr(domain); err != nil && strings.HasSuffix(host, "."+domain) {
					want = true
				}
			}
			if got := idx.matches(host); got != want {
				t.Errorf("host=%s got=%t want=%t", host, got, want)
			}
		}
	}
}
