package network

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"

	dockernetwork "github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
)

const (
	NetworkName   = "dock-net"
	BridgeName    = "dock-net0"
	NetworkSubnet = "10.200.0.0/24"
	NetworkGW     = "10.200.0.1"
)

var ErrDockNetNotFound = errors.New("dock-net does not exist")

// NetworkInfo holds status information about dock-net.
type NetworkInfo struct {
	ID           string
	Driver       string
	ICCDisabled  bool
	IPMasquerade bool
	Subnet       string
}

// FirewallInfo holds status information about dock-net firewall rules.
type FirewallInfo struct {
	Supported                bool
	Root                     bool
	IptablesFound            bool
	ChainExists              bool
	JumpRuleExists           bool
	DockerUserDefaultPolicy  string
	ManagedChainFinalVerdict string
}

// Manager handles the lifecycle of the dock-net Docker network.
type Manager struct {
	cli      *client.Client
	firewall firewallController
}

// EnsureOptions configures dock-net creation and host egress exceptions.
type EnsureOptions struct {
	NoInternet           bool
	AllowHostTCPPorts    []int
	AllowTCPDestinations []HostEndpoint
}

// NewManager creates a new network Manager.
func NewManager() (*Manager, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("connecting to Docker: %w", err)
	}
	return &Manager{cli: cli, firewall: newSystemFirewall()}, nil
}

// EnsureNetwork creates dock-net if it does not already exist.
func (m *Manager) EnsureNetwork(opts EnsureOptions) error {
	ctx := context.Background()

	existing, err := m.findNetwork(ctx)
	if err != nil {
		return err
	}

	if existing == nil {
		options := map[string]string{
			"com.docker.network.bridge.enable_icc":           "false",
			"com.docker.network.bridge.enable_ip_masquerade": "true",
			"com.docker.network.bridge.name":                 BridgeName,
		}

		if opts.NoInternet {
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
					{Subnet: NetworkSubnet, Gateway: NetworkGW},
				},
			},
		})
		if err != nil {
			return fmt.Errorf("creating dock-net: %w", err)
		}
		existing, err = m.findNetwork(ctx)
		if err != nil {
			return err
		}
		if existing == nil {
			return fmt.Errorf("dock-net created but could not be reloaded")
		}
	}

	return nil
}

// ApplyFirewall applies firewall rules to dock-net if possible.
// Returns a warning (non-nil error) for unsupported/non-root environments.
func (m *Manager) ApplyFirewall(opts EnsureOptions) error {
	ctx := context.Background()
	existing, err := m.findNetwork(ctx)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrDockNetNotFound
	}

	if m.firewall == nil {
		return nil
	}

	cfg, err := m.firewallConfig(ctx, opts, existing)
	if err != nil {
		return err
	}
	if err := m.firewall.Apply(ctx, cfg); err != nil {
		if IsFirewallWarning(err) {
			return err
		}
		return fmt.Errorf("applying dock-net firewall rules: %w", err)
	}
	return nil
}

// RemoveFirewall removes firewall rules associated with dock-net.
func (m *Manager) RemoveFirewall() error {
	ctx := context.Background()
	existing, err := m.findNetwork(ctx)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrDockNetNotFound
	}
	if m.firewall == nil {
		return nil
	}

	cfg, err := m.firewallConfig(ctx, EnsureOptions{}, existing)
	if err != nil {
		return err
	}
	if err := m.firewall.Remove(ctx, cfg); err != nil {
		if IsFirewallWarning(err) {
			return err
		}
		return fmt.Errorf("removing dock-net firewall rules: %w", err)
	}
	return nil
}

// FirewallStatus returns information about dock-net firewall rule installation.
func (m *Manager) FirewallStatus() (*FirewallInfo, error) {
	ctx := context.Background()
	existing, err := m.findNetwork(ctx)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrDockNetNotFound
	}
	if m.firewall == nil {
		return &FirewallInfo{}, nil
	}

	cfg, err := m.firewallConfig(ctx, EnsureOptions{}, existing)
	if err != nil {
		return nil, err
	}
	st, err := m.firewall.Status(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("getting dock-net firewall status: %w", err)
	}
	return &FirewallInfo{
		Supported:                st.Supported,
		Root:                     st.Root,
		IptablesFound:            st.IptablesFound,
		ChainExists:              st.ChainExists,
		JumpRuleExists:           st.JumpRuleExists,
		DockerUserDefaultPolicy:  st.DockerUserDefaultPolicy,
		ManagedChainFinalVerdict: st.ManagedChainFinalVerdict,
	}, nil
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
	if m.firewall != nil {
		cfg, err := m.firewallConfig(ctx, EnsureOptions{}, existing)
		if err != nil {
			return err
		}
		if err := m.firewall.Remove(ctx, cfg); err != nil {
			return fmt.Errorf("removing dock-net firewall rules: %w", err)
		}
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
	return m.findNetworkByName(ctx, NetworkName)
}

func (m *Manager) findNetworkByName(ctx context.Context, name string) (*dockernetwork.Summary, error) {
	nets, err := m.cli.NetworkList(ctx, dockernetwork.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing networks: %w", err)
	}
	for i := range nets {
		if nets[i].Name == name {
			return &nets[i], nil
		}
	}
	return nil, nil
}

func (m *Manager) firewallConfig(ctx context.Context, opts EnsureOptions, dockNet *dockernetwork.Summary) (firewallConfig, error) {
	cfg := firewallConfig{
		BridgeName: BridgeName,
	}

	cfg.AllowTCPDestinations = append(cfg.AllowTCPDestinations, normalizeHostEndpoints(opts.AllowTCPDestinations)...)

	if len(opts.AllowHostTCPPorts) == 0 {
		return cfg, nil
	}

	hostIPs := make([]string, 0, 2)
	if gateway := gatewayFromSummary(dockNet); gateway != "" {
		hostIPs = append(hostIPs, gateway)
	}
	if gateway, err := m.hostGatewayAddr(ctx); err == nil && gateway != "" {
		hostIPs = append(hostIPs, gateway)
	}

	for _, port := range opts.AllowHostTCPPorts {
		if port <= 0 || port > 65535 {
			continue
		}
		for _, ip := range hostIPs {
			cfg.AllowTCPDestinations = append(cfg.AllowTCPDestinations, HostEndpoint{IP: ip, Port: port})
		}
	}

	cfg.AllowTCPDestinations = normalizeHostEndpoints(cfg.AllowTCPDestinations)
	return cfg, nil
}

func (m *Manager) hostGatewayAddr(ctx context.Context) (string, error) {
	bridge, err := m.findNetworkByName(ctx, "bridge")
	if err != nil {
		return "", err
	}
	return gatewayFromSummary(bridge), nil
}

func gatewayFromSummary(net *dockernetwork.Summary) string {
	if net == nil {
		return ""
	}
	if len(net.IPAM.Config) == 0 {
		return ""
	}
	if net.IPAM.Config[0].Gateway != "" {
		return net.IPAM.Config[0].Gateway
	}
	if net.IPAM.Config[0].Subnet == "" {
		return ""
	}
	gateway, err := deriveGateway(net.IPAM.Config[0].Subnet)
	if err != nil {
		return ""
	}
	return gateway
}

func AllowHostEndpoint(rawURL string) (HostEndpoint, bool) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return HostEndpoint{}, false
	}
	host := u.Hostname()
	port, err := strconv.Atoi(u.Port())
	if err != nil || net.ParseIP(host) == nil {
		return HostEndpoint{}, false
	}
	return HostEndpoint{IP: host, Port: port}, true
}
