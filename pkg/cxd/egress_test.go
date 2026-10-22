package cxd

import (
	"context"
	"errors"
	"net/netip"
	"testing"

	"github.com/pacificbelt30/codex-dock/internal/networkpolicy"
	"github.com/pacificbelt30/codex-dock/internal/workeregress"
)

func TestEgressPolicyDefaultsAndPublicAccessOptIn(t *testing.T) {
	for _, tc := range []struct {
		name    string
		policy  EgressPolicy
		allowed bool
		offline bool
	}{
		{name: "zero value", offline: true},
		{name: "ports alone", policy: EgressPolicy{AllowPorts: []int{443}}, offline: true},
		{name: "domain grant", policy: EgressPolicy{AllowDomains: []string{"=example.com"}}, allowed: true},
		{name: "public opt in", policy: EgressPolicy{AllowPublicDestinations: true}, allowed: true},
		{name: "deny wins over opt in", policy: EgressPolicy{DefaultDeny: true, AllowPublicDestinations: true}, offline: true},
		{name: "opt in retains deny rules", policy: EgressPolicy{AllowPublicDestinations: true, DenyDomains: []string{"example.com"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rules, offline, err := workeregress.ResolvePreset(tc.policy.rules(), "", nil)
			if err != nil || offline != tc.offline {
				t.Fatalf("resolved offline = %t, %v", offline, err)
			}
			policy, err := networkpolicy.Compile(rules)
			if err != nil {
				t.Fatal(err)
			}
			dst := networkpolicy.Destination{Host: "example.com", Port: 443}
			err = policy.CheckDestination(context.Background(), dst)
			if (err == nil) != tc.allowed {
				t.Fatalf("destination decision = %v, want allowed %t", err, tc.allowed)
			}
			if err := policy.CheckAddress(context.Background(), dst, netip.MustParseAddr("127.0.0.1")); !errors.Is(err, networkpolicy.ErrDenied) {
				t.Fatal("private address permitted", err)
			}
			if tc.offline {
				if _, _, err := workeregress.ResolvePreset(rules, "", []string{"=example.com"}); err == nil {
					t.Fatal("worker widened empty host grants")
				}
			}
		})
	}
}
