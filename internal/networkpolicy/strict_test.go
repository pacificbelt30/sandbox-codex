package networkpolicy

import (
	"context"
	"errors"
	"net/netip"
	"testing"
)

func TestDefaultDenyRequiresDestinationGrant(t *testing.T) {
	for _, rules := range []Rules{
		{DefaultDeny: true},
		{DefaultDeny: true, AllowPorts: []int{443}},
		{DefaultDeny: true, DenyDomains: []string{"other.example"}},
	} {
		p, err := Compile(rules)
		if err != nil {
			t.Fatal(err)
		}
		d := Destination{Host: "example.com", Port: 443}
		if err := p.CheckDestination(context.Background(), d); !errors.Is(err, ErrDenied) || Reason(err) != "default_denied" {
			t.Fatalf("empty strict policy granted a destination: %+v %v", rules, err)
		}
		if err := p.CheckAddress(context.Background(), d, netip.MustParseAddr("203.0.113.1")); !errors.Is(err, ErrDenied) {
			t.Fatal("empty strict policy granted an address")
		}
	}
	for _, rules := range []Rules{
		{}, // Legacy empty lists retain their documented meaning.
		{DefaultDeny: true, AllowDomains: []string{"=example.com"}},
		{DefaultDeny: true, AllowCIDRs: []string{"203.0.113.0/24"}},
	} {
		p, err := Compile(rules)
		if err != nil {
			t.Fatal(err)
		}
		d := Destination{Host: "example.com", Port: 443}
		if err := p.CheckDestination(context.Background(), d); err != nil {
			t.Fatal(err)
		}
		if err := p.CheckAddress(context.Background(), d, netip.MustParseAddr("203.0.113.1")); err != nil {
			t.Fatal(err)
		}
	}
}

func TestExactAndSubdomainRules(t *testing.T) {
	for _, tc := range []struct {
		rules Rules
		host  string
		allow bool
	}{
		{Rules{AllowDomains: []string{"=API.Example.COM."}}, "api.example.com", true},
		{Rules{AllowDomains: []string{"=api.example.com"}}, "sub.api.example.com", false},
		{Rules{AllowDomains: []string{"api.example.com"}}, "sub.api.example.com", true},
		{Rules{AllowDomains: []string{"example.com"}, DenyDomains: []string{"=api.example.com"}}, "api.example.com", false},
		{Rules{AllowDomains: []string{"example.com"}, DenyDomains: []string{"=api.example.com"}}, "sub.api.example.com", true},
		{Rules{AllowDomains: []string{"=api.example.com"}, DenyDomains: []string{"example.com"}}, "api.example.com", false},
	} {
		p, err := Compile(tc.rules)
		if err != nil {
			t.Fatal(err)
		}
		err = p.CheckDestination(context.Background(), Destination{Host: tc.host, Port: 443})
		if (err == nil) != tc.allow {
			t.Fatalf("rules=%+v host=%s error=%v", tc.rules, tc.host, err)
		}
	}
	for _, value := range []string{"=", "=.example.com", "==example.com", "=*.example.com", "=example.com/path"} {
		if _, err := Compile(Rules{AllowDomains: []string{value}}); err == nil {
			t.Fatalf("invalid exact spelling accepted: %q", value)
		}
	}
}

func TestDomainSubsetIncludesMatchingModeAndIPSemantics(t *testing.T) {
	for _, tc := range []struct {
		child, parent string
		within        bool
	}{
		{"=api.example.com", "example.com", true},
		{"api.example.com", "=api.example.com", false},
		{"=api.example.com", "=API.Example.COM.", true},
		{"=sub.api.example.com", "=api.example.com", false},
		{"other.example", "example.com", false},
		{"example.com", "example.com.", true},
		{"192.0.2.1", "=192.0.2.1", true},
		{"foo.192.0.2.1", "192.0.2.1", false},
	} {
		within, err := DomainWithin(tc.child, tc.parent)
		if err != nil || within != tc.within {
			t.Fatalf("child=%s parent=%s within=%t error=%v", tc.child, tc.parent, within, err)
		}
	}
}
