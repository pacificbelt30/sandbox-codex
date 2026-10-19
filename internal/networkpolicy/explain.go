package networkpolicy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
)

// Denial is a machine-readable policy failure. It contains no URL paths,
// request headers, credentials, or resolved private IP addresses.
type Denial struct {
	Code        string      `json:"code"`
	Destination Destination `json:"destination"`
	Rule        string      `json:"rule,omitempty"`
}

func (e *Denial) Error() string { return fmt.Sprintf("%s: %s (%s)", ErrDenied, e.Destination, e.Code) }
func (e *Denial) Unwrap() error { return ErrDenied }

func Reason(err error) string {
	var denied *Denial
	if errors.As(err, &denied) {
		return denied.Code
	}
	if errors.Is(err, ErrDenied) {
		return "policy_denied"
	}
	return "connection_failed"
}

type Decision struct {
	Destination      Destination `json:"destination"`
	Allowed          bool        `json:"allowed"`
	Code             string      `json:"code"`
	Stage            string      `json:"stage"`
	AddressesChecked int         `json:"addresses_checked"`
	Complete         bool        `json:"complete"`
}

// Explain performs a connection-free preflight. Without resolve, a positive
// result covers the name and port only; it never claims the DNS checks passed.
func Explain(ctx context.Context, rules Rules, authority string, resolve bool, resolver Resolver) (Decision, error) {
	policy, err := Compile(rules)
	if err != nil {
		return Decision{}, err
	}
	d, err := ParseDestination(authority, "443")
	if err != nil {
		return Decision{}, err
	}
	result := Decision{Destination: d, Stage: "destination", Code: "address_not_checked"}
	if err := policy.CheckDestination(ctx, d); err != nil {
		if !errors.Is(err, ErrDenied) {
			return result, err
		}
		result.Code, result.Complete = Reason(err), true
		return result, nil
	}
	if !resolve {
		result.Allowed = true
		return result, nil
	}
	addresses := []netip.Addr{}
	if ip, err := netip.ParseAddr(d.Host); err == nil {
		addresses = append(addresses, ip)
	} else {
		if resolver == nil {
			resolver = net.DefaultResolver
		}
		addresses, err = resolver.LookupNetIP(ctx, "ip", d.Host)
		if err != nil {
			return result, fmt.Errorf("resolving destination: %w", err)
		}
	}
	result.Stage = "address"
	if len(addresses) == 0 {
		return result, fmt.Errorf("destination has no addresses")
	}
	for _, ip := range addresses {
		result.AddressesChecked++
		if err := policy.CheckAddress(ctx, d, ip); err != nil {
			if !errors.Is(err, ErrDenied) {
				return result, err
			}
			result.Code, result.Complete = Reason(err), true
			return result, nil
		}
	}
	result.Allowed, result.Complete, result.Code = true, true, "allowed"
	return result, nil
}
