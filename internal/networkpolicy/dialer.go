package networkpolicy

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"time"
)

type Resolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}

// Dialer validates the whole DNS answer, then dials a checked IP directly. DNS
// is never performed again by the underlying connector (prevents rebinding).
type Dialer struct {
	Policy   Policy
	Resolver Resolver
	Connect  func(context.Context, string, string) (net.Conn, error)
}

func (d Dialer) DialContext(ctx context.Context, network, authority string) (net.Conn, error) {
	dst, err := ParseDestination(authority, "")
	if err != nil {
		return nil, err
	}
	if d.Policy == nil {
		return nil, fmt.Errorf("network policy is required")
	}
	if err := d.Policy.CheckDestination(ctx, dst); err != nil {
		return nil, err
	}
	resolver := d.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	var addresses []netip.Addr
	if ip, err := netip.ParseAddr(dst.Host); err == nil {
		addresses = []netip.Addr{ip}
	} else {
		addresses, err = resolver.LookupNetIP(ctx, "ip", dst.Host)
		if err != nil {
			return nil, err
		}
	}
	if len(addresses) == 0 {
		return nil, fmt.Errorf("no address for %s", dst.Host)
	}
	for _, ip := range addresses {
		if err := d.Policy.CheckAddress(ctx, dst, ip); err != nil {
			return nil, err
		}
	}
	connect := d.Connect
	if connect == nil {
		connect = (&net.Dialer{Timeout: 30 * time.Second}).DialContext
	}
	var lastErr error
	for _, ip := range addresses {
		address := net.JoinHostPort(ip.Unmap().String(), fmt.Sprint(dst.Port))
		conn, err := connect(ctx, network, address)
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	return nil, lastErr
}
