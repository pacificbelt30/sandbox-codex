package cxd

import "github.com/pacificbelt30/codex-dock/internal/networkpolicy"

// EgressPolicy is the host product's maximum general-egress policy. A worker
// domain allowlist can only narrow this policy and never permits private IPs.
type EgressPolicy struct {
	AllowDomains []string
	DenyDomains  []string
	AllowCIDRs   []string
	DenyCIDRs    []string
	AllowPorts   []int
	DenyPorts    []int
	// DefaultDeny enforces destination grants even if AllowPublicDestinations
	// is true. Zero-value policies already deny egress without domain/CIDR grants.
	DefaultDeny bool
	// AllowPublicDestinations opts into public-destination access without domain
	// or CIDR grants. Private addresses and all explicit restrictions still apply.
	AllowPublicDestinations bool
}

func (p EgressPolicy) rules() networkpolicy.Rules {
	return networkpolicy.Rules{
		AllowDomains: p.AllowDomains, DenyDomains: p.DenyDomains,
		AllowCIDRs: p.AllowCIDRs, DenyCIDRs: p.DenyCIDRs,
		AllowPorts: p.AllowPorts, DenyPorts: p.DenyPorts,
		BlockPrivate: true,
		DefaultDeny:  p.DefaultDeny || !p.AllowPublicDestinations,
	}
}
