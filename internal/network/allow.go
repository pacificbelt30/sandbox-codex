package network

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// ParseHostEndpoint parses an "IP:PORT" allow-list entry into a HostEndpoint.
//
// Only literal IP addresses are accepted (the firewall operates on IPs, not
// hostnames). IPv6 literals must be bracketed, e.g. "[::1]:8080".
func ParseHostEndpoint(spec string) (HostEndpoint, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return HostEndpoint{}, fmt.Errorf("empty host endpoint")
	}

	host, portStr, err := net.SplitHostPort(spec)
	if err != nil {
		return HostEndpoint{}, fmt.Errorf("invalid host endpoint %q (expected IP:PORT): %w", spec, err)
	}
	if net.ParseIP(host) == nil {
		return HostEndpoint{}, fmt.Errorf("invalid host endpoint %q: %q is not an IP address", spec, host)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > 65535 {
		return HostEndpoint{}, fmt.Errorf("invalid host endpoint %q: port must be 1-65535", spec)
	}
	return HostEndpoint{IP: host, Port: port}, nil
}

// ParseHostEndpoints parses a list of "IP:PORT" allow-list entries.
// It fails on the first invalid entry so the user gets immediate feedback
// rather than silently dropping a destination they expected to be allowed.
func ParseHostEndpoints(specs []string) ([]HostEndpoint, error) {
	endpoints := make([]HostEndpoint, 0, len(specs))
	for _, spec := range specs {
		ep, err := ParseHostEndpoint(spec)
		if err != nil {
			return nil, err
		}
		endpoints = append(endpoints, ep)
	}
	return endpoints, nil
}
