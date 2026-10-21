package workeregress

import (
	"fmt"
	"github.com/pacificbelt30/codex-dock/internal/networkpolicy"
)

type Preset struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Offline     bool     `json:"offline"`
	Domains     []string `json:"domains,omitempty"`
}

func Presets() []Preset {
	return []Preset{
		{Name: "offline", Description: "No general egress; fixed AI routes remain available", Offline: true},
		{Name: "test", Description: "Offline tests; fixed AI routes remain available", Offline: true},
		{Name: "dependencies", Description: "Common package registries; host policy remains authoritative", Domains: []string{"registry.npmjs.org", "pypi.org", "files.pythonhosted.org", "proxy.golang.org", "sum.golang.org"}},
		{Name: "api", Description: "Only explicitly supplied API domains"},
	}
}

// ResolvePreset is opt-in. Empty preserves existing empty-allowlist semantics.
func ResolvePreset(host networkpolicy.Rules, name string, domains []string) (networkpolicy.Rules, bool, error) {
	if name == "" {
		if len(domains) == 0 {
			_, err := networkpolicy.Compile(host)
			return host, host.DefaultDeny && len(host.AllowDomains) == 0 && len(host.AllowCIDRs) == 0, err
		}
		r, err := Rules(host, domains)
		return r, false, err
	}
	for _, p := range Presets() {
		if p.Name != name {
			continue
		}
		if p.Offline {
			if len(domains) > 0 {
				return networkpolicy.Rules{}, false, fmt.Errorf("offline preset cannot grant domains")
			}
			return host, true, nil
		}
		if name == "dependencies" && len(domains) == 0 {
			domains = p.Domains
		}
		if len(domains) == 0 {
			return networkpolicy.Rules{}, false, fmt.Errorf("api preset requires explicit allowed domains")
		}
		r, err := Rules(host, domains)
		return r, false, err
	}
	return networkpolicy.Rules{}, false, fmt.Errorf("unknown policy preset %q", name)
}
