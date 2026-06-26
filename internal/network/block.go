package network

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// ParseBlockDestination parses a --block-host entry into a BlockDestination.
//
// Accepted forms (IPv4 only — the firewall uses iptables):
//
//	CIDR     e.g. "203.0.113.0/24"   -> drop all traffic to the range
//	IP       e.g. "203.0.113.10"     -> drop all traffic to that /32 host
//	IP:PORT  e.g. "203.0.113.10:443" -> drop only TCP traffic to that port
func ParseBlockDestination(spec string) (BlockDestination, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return BlockDestination{}, fmt.Errorf("empty block destination")
	}

	// CIDR form.
	if strings.Contains(spec, "/") {
		ip, ipnet, err := net.ParseCIDR(spec)
		if err != nil {
			return BlockDestination{}, fmt.Errorf("invalid block destination %q (expected CIDR/IP/IP:PORT): %w", spec, err)
		}
		if ip.To4() == nil {
			return BlockDestination{}, fmt.Errorf("invalid block destination %q: only IPv4 is supported", spec)
		}
		return BlockDestination{CIDR: ipnet.String(), Port: 0}, nil
	}

	// IP:PORT form.
	if host, portStr, err := net.SplitHostPort(spec); err == nil {
		ip := net.ParseIP(host)
		if ip == nil || ip.To4() == nil {
			return BlockDestination{}, fmt.Errorf("invalid block destination %q: %q is not an IPv4 address", spec, host)
		}
		port, err := strconv.Atoi(portStr)
		if err != nil || port <= 0 || port > 65535 {
			return BlockDestination{}, fmt.Errorf("invalid block destination %q: port must be 1-65535", spec)
		}
		return BlockDestination{CIDR: host + "/32", Port: port}, nil
	}

	// Bare IP form.
	ip := net.ParseIP(spec)
	if ip == nil || ip.To4() == nil {
		return BlockDestination{}, fmt.Errorf("invalid block destination %q: expected an IPv4 CIDR, IP, or IP:PORT", spec)
	}
	return BlockDestination{CIDR: spec + "/32", Port: 0}, nil
}

// ParseBlockDestinations parses a list of --block-host entries, failing on the
// first invalid entry so the user gets immediate feedback.
func ParseBlockDestinations(specs []string) ([]BlockDestination, error) {
	blocks := make([]BlockDestination, 0, len(specs))
	for _, spec := range specs {
		b, err := ParseBlockDestination(spec)
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, b)
	}
	return blocks, nil
}
