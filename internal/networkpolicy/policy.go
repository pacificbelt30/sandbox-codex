// Package networkpolicy evaluates destinations independently of HTTP, credentials
// and Docker. Policies are immutable and safe to share between requests.
package networkpolicy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
)

var ErrDenied = errors.New("destination denied by network policy")

// Destination is the canonical authority requested by a client. HTTP paths and
// credentials are deliberately excluded from policy decisions and errors.
type Destination struct {
	Host string
	Port uint16
}

func ParseDestination(authority, defaultPort string) (Destination, error) {
	host, port, err := net.SplitHostPort(authority)
	if err != nil {
		if defaultPort == "" {
			return Destination{}, fmt.Errorf("invalid destination authority")
		}
		host, port = authority, defaultPort
		if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
			host = host[1 : len(host)-1]
		} else if strings.ContainsAny(host, ":[]") {
			return Destination{}, fmt.Errorf("invalid destination authority")
		}
	}
	host, err = normalizeHost(host)
	if err != nil {
		return Destination{}, err
	}
	n, err := strconv.ParseUint(port, 10, 16)
	if err != nil || n == 0 {
		return Destination{}, fmt.Errorf("invalid destination port")
	}
	return Destination{Host: host, Port: uint16(n)}, nil
}

func (d Destination) String() string { return net.JoinHostPort(d.Host, strconv.Itoa(int(d.Port))) }

// Policy is the extension point for configuration-backed or custom policies.
// CheckDestination runs before DNS; CheckAddress checks every resolved address
// before ANY connection is made. Implementations must support concurrent calls.
type Policy interface {
	CheckDestination(context.Context, Destination) error
	CheckAddress(context.Context, Destination, netip.Addr) error
}

type Rules struct {
	AllowDomains []string
	DenyDomains  []string
	AllowCIDRs   []string
	DenyCIDRs    []string
	AllowPorts   []int
	DenyPorts    []int
	BlockPrivate bool
	// DefaultDeny requires a domain or CIDR grant. Ports alone never grant a
	// destination. Omission retains the legacy public-destination behavior.
	DefaultDeny bool `json:",omitempty"`
}

type compiled struct {
	allowDomains, denyDomains         []string
	allowDomainIndex, denyDomainIndex domainIndex
	allowCIDRs, denyCIDRs             []netip.Prefix
	allowPorts, denyPorts             map[uint16]bool
	blockPrivate                      bool
	defaultDeny                       bool
}

// Compile validates all rules up front. Denials override allowances; nonempty
// domain, CIDR and port allowlists are intersected. In DefaultDeny mode, empty
// domain AND CIDR allowlists deny all general destinations, even with ports set.
func Compile(r Rules) (Policy, error) {
	p := &compiled{blockPrivate: r.BlockPrivate, defaultDeny: r.DefaultDeny}
	for _, pair := range []struct {
		input  []string
		output *[]string
	}{{r.AllowDomains, &p.allowDomains}, {r.DenyDomains, &p.denyDomains}} {
		for _, value := range pair.input {
			host, exact, err := parseDomain(value)
			if err != nil {
				return nil, fmt.Errorf("invalid policy domain %q: %w", value, err)
			}
			if exact {
				host = "=" + host
			}
			*pair.output = append(*pair.output, host)
		}
	}
	p.allowDomainIndex = indexDomains(p.allowDomains)
	p.denyDomainIndex = indexDomains(p.denyDomains)
	for _, pair := range []struct {
		input  []string
		output *[]netip.Prefix
	}{{r.AllowCIDRs, &p.allowCIDRs}, {r.DenyCIDRs, &p.denyCIDRs}} {
		for _, value := range pair.input {
			prefix, err := netip.ParsePrefix(value)
			if err != nil {
				ip, ipErr := netip.ParseAddr(value)
				if ipErr != nil || ip.Zone() != "" {
					return nil, fmt.Errorf("invalid policy IP/CIDR %q", value)
				}
				prefix = netip.PrefixFrom(ip, ip.BitLen())
			}
			if prefix.Addr().Is4In6() {
				if prefix.Bits() < 96 {
					return nil, fmt.Errorf("ambiguous mapped IPv4 CIDR %q", value)
				}
				prefix = netip.PrefixFrom(prefix.Addr().Unmap(), prefix.Bits()-96)
			}
			*pair.output = append(*pair.output, prefix.Masked())
		}
	}
	for _, pair := range []struct {
		input  []int
		output *map[uint16]bool
	}{{r.AllowPorts, &p.allowPorts}, {r.DenyPorts, &p.denyPorts}} {
		*pair.output = make(map[uint16]bool)
		for _, port := range pair.input {
			if port < 1 || port > 65535 {
				return nil, fmt.Errorf("invalid policy port %d", port)
			}
			(*pair.output)[uint16(port)] = true
		}
	}
	return p, nil
}

func (p *compiled) CheckDestination(ctx context.Context, d Destination) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	host, err := normalizeHost(d.Host)
	if err != nil || d.Port == 0 {
		return fmt.Errorf("%w: invalid authority", ErrDenied)
	}
	if p.denyDomainIndex.matches(host) {
		return &Denial{Code: "domain_denied", Destination: d}
	}
	if p.denyPorts[d.Port] {
		return &Denial{Code: "port_denied", Destination: d}
	}
	if p.defaultDeny && len(p.allowDomains) == 0 && len(p.allowCIDRs) == 0 {
		return &Denial{Code: "default_denied", Destination: d}
	}
	if len(p.allowDomains) > 0 && !p.allowDomainIndex.matches(host) {
		return &Denial{Code: "domain_not_allowed", Destination: d}
	}
	if len(p.allowPorts) > 0 && !p.allowPorts[d.Port] {
		return &Denial{Code: "port_not_allowed", Destination: d}
	}
	return nil
}

func (p *compiled) CheckAddress(ctx context.Context, d Destination, ip netip.Addr) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	ip = ip.Unmap()
	if p.defaultDeny && len(p.allowDomains) == 0 && len(p.allowCIDRs) == 0 {
		return &Denial{Code: "default_denied", Destination: d}
	}
	if !ip.IsValid() || ip.Zone() != "" {
		return &Denial{Code: "invalid_address", Destination: d}
	}
	if p.blockPrivate && IsPrivate(ip) {
		return &Denial{Code: "private_address", Destination: d}
	}
	if matchesCIDR(ip, p.denyCIDRs) {
		return &Denial{Code: "address_denied", Destination: d}
	}
	if len(p.allowCIDRs) > 0 && !matchesCIDR(ip, p.allowCIDRs) {
		return &Denial{Code: "address_not_allowed", Destination: d}
	}
	return nil
}

func normalizeHost(host string) (string, error) {
	// Ordinary DNS names cannot be IP literals; avoid allocating a ParseAddr error.
	if strings.Contains(host, ":") || len(host) > 0 && host[0] >= '0' && host[0] <= '9' {
		if ip, err := netip.ParseAddr(host); err == nil && ip.Zone() == "" {
			return ip.Unmap().String(), nil
		}
	}
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "" || len(host) > 253 || strings.HasSuffix(host, ".") {
		return "", fmt.Errorf("invalid destination host")
	}
	for remaining := host; remaining != ""; {
		label, rest, _ := strings.Cut(remaining, ".")
		remaining = rest
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", fmt.Errorf("invalid destination host")
		}
		for _, c := range label {
			switch {
			case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-':
			default:
				return "", fmt.Errorf("invalid destination host")
			}
		}
	}
	return host, nil
}

// DNS suffixes are compiled once. Lookup is bounded by the number of labels
// in a destination, rather than the size of the configured allow/deny list.
type domainIndex struct{ exact, suffix map[string]bool }

func indexDomains(domains []string) domainIndex {
	index := domainIndex{exact: make(map[string]bool, len(domains)), suffix: make(map[string]bool, len(domains))}
	for _, domain := range domains {
		domain, exact := strings.CutPrefix(domain, "=")
		index.exact[domain] = true
		if _, err := netip.ParseAddr(domain); err != nil && !exact {
			index.suffix[domain] = true
		}
	}
	return index
}

// DomainWithin reports whether every host matched by child is also matched by
// parent. Bare names include subdomains; =name is exact. IP literals are exact.
func DomainWithin(child, parent string) (bool, error) {
	c, childExact, err := parseDomain(child)
	if err != nil {
		return false, err
	}
	p, parentExact, err := parseDomain(parent)
	if err != nil {
		return false, err
	}
	if _, err := netip.ParseAddr(p); err == nil {
		parentExact = true
	}
	if _, err := netip.ParseAddr(c); err == nil {
		childExact = true
	}
	if parentExact {
		return childExact && c == p, nil
	}
	return c == p || strings.HasSuffix(c, "."+p), nil
}

func parseDomain(value string) (string, bool, error) {
	value = strings.TrimSpace(value)
	host, exact := strings.CutPrefix(value, "=")
	if !exact {
		host = strings.TrimPrefix(host, ".") // Legacy spelling includes subdomains.
	}
	host, err := normalizeHost(host)
	return host, exact, err
}
func (index domainIndex) matches(host string) bool {
	if index.exact[host] {
		return true
	}
	for {
		i := strings.IndexByte(host, '.')
		if i < 0 {
			return false
		}
		host = host[i+1:]
		if index.suffix[host] {
			return true
		}
	}
}

func matchesCIDR(ip netip.Addr, prefixes []netip.Prefix) bool {
	for _, prefix := range prefixes {
		if prefix.Contains(ip) {
			return true
		}
	}
	return false
}

// IsPrivate includes LAN, loopback, metadata/link-local and CGNAT destinations.
var carrierGradeNAT = netip.MustParsePrefix("100.64.0.0/10")

func IsPrivate(ip netip.Addr) bool {
	ip = ip.Unmap()
	return !ip.IsValid() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || carrierGradeNAT.Contains(ip)
}

type combined []Policy

// All composes policies without allowing an extension to weaken base rules.
func All(policies ...Policy) Policy { return combined(policies) }
func (p combined) CheckDestination(ctx context.Context, d Destination) error {
	for _, policy := range p {
		if policy != nil {
			if err := policy.CheckDestination(ctx, d); err != nil {
				return err
			}
		}
	}
	return nil
}
func (p combined) CheckAddress(ctx context.Context, d Destination, ip netip.Addr) error {
	for _, policy := range p {
		if policy != nil {
			if err := policy.CheckAddress(ctx, d, ip); err != nil {
				return err
			}
		}
	}
	return nil
}
