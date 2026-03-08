package network

import (
	"context"
	"fmt"

	dockernetwork "github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
)

const (
	NetworkName = "dock-net"
)

// NetworkInfo holds status information about dock-net.
type NetworkInfo struct {
	ID           string
	Driver       string
	ICCDisabled  bool
	IPMasquerade bool
	Subnet       string
}

// Manager handles the lifecycle of the dock-net Docker network.
type Manager struct {
	cli *client.Client
}

// NewManager creates a new network Manager.
func NewManager() (*Manager, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("connecting to Docker: %w", err)
	}
	return &Manager{cli: cli}, nil
}

// EnsureNetwork creates dock-net if it does not already exist.
func (m *Manager) EnsureNetwork(noInternet bool) error {
	ctx := context.Background()

	existing, err := m.findNetwork(ctx)
	if err != nil {
		return err
	}
	if existing != nil {
		return nil // already exists
	}

	options := map[string]string{
		"com.docker.network.bridge.enable_icc":           "false",
		"com.docker.network.bridge.enable_ip_masquerade": "true",
		"com.docker.network.bridge.name":                 "dock-net0",
	}

	if noInternet {
		options["com.docker.network.bridge.enable_ip_masquerade"] = "false"
	}

	_, err = m.cli.NetworkCreate(ctx, NetworkName, dockernetwork.CreateOptions{
		Driver:  "bridge",
		Options: options,
		Labels: map[string]string{
			"codex-dock.managed": "true",
		},
		IPAM: &dockernetwork.IPAM{
			Driver: "default",
			Config: []dockernetwork.IPAMConfig{
				{Subnet: "10.200.0.0/24", Gateway: "10.200.0.1"},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("creating dock-net: %w", err)
	}
	return nil
}

// RemoveNetwork removes dock-net.
func (m *Manager) RemoveNetwork() error {
	ctx := context.Background()
	existing, err := m.findNetwork(ctx)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("dock-net does not exist")
	}
	return m.cli.NetworkRemove(ctx, existing.ID)
}

// Status returns information about dock-net, or nil if it doesn't exist.
func (m *Manager) Status() (*NetworkInfo, error) {
	ctx := context.Background()
	net, err := m.findNetwork(ctx)
	if err != nil {
		return nil, err
	}
	if net == nil {
		return nil, nil
	}

	info := &NetworkInfo{
		ID:     net.ID,
		Driver: net.Driver,
	}

	if v, ok := net.Options["com.docker.network.bridge.enable_icc"]; ok {
		info.ICCDisabled = v == "false"
	}
	if v, ok := net.Options["com.docker.network.bridge.enable_ip_masquerade"]; ok {
		info.IPMasquerade = v == "true"
	}
	if len(net.IPAM.Config) > 0 {
		info.Subnet = net.IPAM.Config[0].Subnet
	}

	return info, nil
}

// GatewayAddr returns the gateway IP address of dock-net.
// Containers on dock-net use this address to reach the host (and the Auth Proxy).
// Returns an error if dock-net does not exist or gateway cannot be determined.
func (m *Manager) GatewayAddr() (string, error) {
	ctx := context.Background()
	nets, err := m.cli.NetworkList(ctx, dockernetwork.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("listing networks: %w", err)
	}
	for _, n := range nets {
		if n.Name != NetworkName {
			continue
		}
		// Try gateway from list result first.
		if len(n.IPAM.Config) > 0 && n.IPAM.Config[0].Gateway != "" {
			return n.IPAM.Config[0].Gateway, nil
		}
		// Inspect for fuller IPAM data — the list endpoint may omit Gateway.
		detail, err := m.cli.NetworkInspect(ctx, n.ID, dockernetwork.InspectOptions{})
		if err == nil && len(detail.IPAM.Config) > 0 && detail.IPAM.Config[0].Gateway != "" {
			return detail.IPAM.Config[0].Gateway, nil
		}
		// Last resort: derive first host address from subnet (e.g. 10.200.0.1 from .0/24).
		if len(n.IPAM.Config) > 0 {
			return deriveGateway(n.IPAM.Config[0].Subnet)
		}
	}
	return "", fmt.Errorf("dock-net not found")
}

// deriveGateway computes the first host address of an IPv4 CIDR subnet.
// For example "10.200.0.0/24" → "10.200.0.1".
func deriveGateway(cidr string) (string, error) {
	b, err := parseIPv4Network(cidr)
	if err != nil {
		return "", fmt.Errorf("parsing subnet %q: %w", cidr, err)
	}
	// Increment last byte to get the first host address.
	for i := len(b) - 1; i >= 0; i-- {
		b[i]++
		if b[i] != 0 {
			break
		}
	}
	return fmt.Sprintf("%d.%d.%d.%d", b[0], b[1], b[2], b[3]), nil
}

// parseIPv4Network extracts the 4-byte network address from an IPv4 CIDR string.
func parseIPv4Network(cidr string) ([4]byte, error) {
	var b [4]byte
	// Find the '/' separator.
	slash := -1
	for i, c := range cidr {
		if c == '/' {
			slash = i
			break
		}
	}
	if slash < 0 {
		return b, fmt.Errorf("no prefix length in CIDR")
	}
	octets := cidr[:slash]
	n := 0
	cur := 0
	for _, c := range octets + "." {
		if c == '.' {
			if n >= 4 {
				return b, fmt.Errorf("too many octets")
			}
			b[n] = byte(cur)
			n++
			cur = 0
		} else if c >= '0' && c <= '9' {
			cur = cur*10 + int(c-'0')
			if cur > 255 {
				return b, fmt.Errorf("octet out of range")
			}
		} else {
			return b, fmt.Errorf("invalid character %q", c)
		}
	}
	if n != 4 {
		return b, fmt.Errorf("expected 4 octets, got %d", n)
	}
	return b, nil
}

func (m *Manager) findNetwork(ctx context.Context) (*dockernetwork.Summary, error) {
	nets, err := m.cli.NetworkList(ctx, dockernetwork.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing networks: %w", err)
	}
	for i := range nets {
		if nets[i].Name == NetworkName {
			return &nets[i], nil
		}
	}
	return nil, nil
}
