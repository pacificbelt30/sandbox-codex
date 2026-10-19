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
}

func (p EgressPolicy) rules() networkpolicy.Rules {
	return networkpolicy.Rules{
		AllowDomains: p.AllowDomains, DenyDomains: p.DenyDomains,
		AllowCIDRs: p.AllowCIDRs, DenyCIDRs: p.DenyCIDRs,
		AllowPorts: p.AllowPorts, DenyPorts: p.DenyPorts,
		BlockPrivate: true,
	}
}
