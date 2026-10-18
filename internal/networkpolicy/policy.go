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
}

type compiled struct {
	allowDomains, denyDomains []string
	allowCIDRs, denyCIDRs     []netip.Prefix
	allowPorts, denyPorts     map[uint16]bool
	blockPrivate              bool
}

// Compile validates all rules up front. Denials override allowances; nonempty
// domain, CIDR and port allowlists are intersected. Empty lists add no limit.
func Compile(r Rules) (Policy, error) {
	p := &compiled{blockPrivate: r.BlockPrivate}
	for _, pair := range []struct {
		input  []string
		output *[]string
	}{{r.AllowDomains, &p.allowDomains}, {r.DenyDomains, &p.denyDomains}} {
		for _, value := range pair.input {
			host, err := normalizeHost(strings.TrimPrefix(strings.TrimSpace(value), "."))
			if err != nil {
				return nil, fmt.Errorf("invalid policy domain %q: %w", value, err)
			}
			*pair.output = append(*pair.output, host)
		}
	}
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
	if matchesDomain(host, p.denyDomains) || p.denyPorts[d.Port] ||
		(len(p.allowDomains) > 0 && !matchesDomain(host, p.allowDomains)) ||
		(len(p.allowPorts) > 0 && !p.allowPorts[d.Port]) {
		return fmt.Errorf("%w: %s", ErrDenied, d)
	}
	return nil
}

func (p *compiled) CheckAddress(ctx context.Context, d Destination, ip netip.Addr) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	ip = ip.Unmap()
	if !ip.IsValid() || ip.Zone() != "" || (p.blockPrivate && IsPrivate(ip)) || matchesCIDR(ip, p.denyCIDRs) || (len(p.allowCIDRs) > 0 && !matchesCIDR(ip, p.allowCIDRs)) {
		return fmt.Errorf("%w: %s resolves to a blocked address", ErrDenied, d)
	}
	return nil
}

func normalizeHost(host string) (string, error) {
	if ip, err := netip.ParseAddr(host); err == nil && ip.Zone() == "" {
		return ip.Unmap().String(), nil
	}
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "" || len(host) > 253 {
		return "", fmt.Errorf("invalid destination host")
	}
	for _, label := range strings.Split(host, ".") {
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

func matchesDomain(host string, domains []string) bool {
	for _, domain := range domains {
		if host == domain {
			return true
		}
		if _, err := netip.ParseAddr(domain); err != nil && strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
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
func IsPrivate(ip netip.Addr) bool {
	ip = ip.Unmap()
	return !ip.IsValid() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || netip.MustParsePrefix("100.64.0.0/10").Contains(ip)
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
