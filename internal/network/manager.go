package network

import (
	"context"
	"fmt"
	"strings"

	dockernetwork "github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
)

const (
	// EgressNetworkName is the bridge network that gives the auth proxy container
	// outbound internet access (NAT/masquerade enabled by default). Worker
	// containers are never attached to it; only the proxy is.
	EgressNetworkName = "dock-net-proxy"
	// EgressBridgeName is the Linux bridge backing the egress network.
	EgressBridgeName = "dock-net-proxy0"

	// WorkerNetPrefix namespaces the per-worker internal networks. Each worker
	// gets its own dedicated internal bridge shared only with the proxy, so
	// workers cannot reach each other (separate L2 segments) and cannot reach
	// the host or internet directly (Internal: true → no NAT, no host route).
	WorkerNetPrefix = "dock-net-w-"

	managedLabel = "codex-dock.managed"
)

// Backwards-compatible aliases: the "proxy network" is the egress network.
const (
	ProxyNetworkName = EgressNetworkName
	ProxyBridgeName  = EgressBridgeName
)

// NetworkInfo holds status information about a Docker network managed by codex-dock.
type NetworkInfo struct {
	ID       string
	Driver   string
	Internal bool
	Subnet   string
}

// Manager handles the lifecycle of codex-dock's Docker networks: the shared
// egress network for the proxy and the per-worker internal networks.
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

// WorkerNetworkName returns the per-worker internal network name for a worker.
func WorkerNetworkName(worker string) string {
	return WorkerNetPrefix + worker
}

// EnsureEgressNetwork creates the egress (proxy) bridge network if it does not
// already exist. Masquerade is left at the Docker default (enabled), so the
// proxy container attached to it reaches the internet.
func (m *Manager) EnsureEgressNetwork() error {
	ctx := context.Background()
	existing, err := m.findNetworkByName(ctx, EgressNetworkName)
	if err != nil {
		return err
	}
	if existing != nil {
		return nil
	}
	_, err = m.cli.NetworkCreate(ctx, EgressNetworkName, dockernetwork.CreateOptions{
		Driver: "bridge",
		Options: map[string]string{
			"com.docker.network.bridge.name": EgressBridgeName,
		},
		Labels: map[string]string{managedLabel: "true"},
	})
	if err != nil {
		return fmt.Errorf("creating %s: %w", EgressNetworkName, err)
	}
	return nil
}

// EnsureWorkerNetwork creates a per-worker internal bridge network if it does
// not already exist. The network is Internal (no NAT, no host route) so the
// worker's only reachable peer is the proxy once it is connected.
func (m *Manager) EnsureWorkerNetwork(name string) error {
	ctx := context.Background()
	netName := WorkerNetworkName(name)
	existing, err := m.findNetworkByName(ctx, netName)
	if err != nil {
		return err
	}
	if existing != nil {
		return nil
	}
	// No fixed subnet: let Docker's default address pool assign one so many
	// concurrent workers do not collide. ICC is left at the default (enabled)
	// because only the proxy and a single worker ever share this network.
	_, err = m.cli.NetworkCreate(ctx, netName, dockernetwork.CreateOptions{
		Driver:   "bridge",
		Internal: true,
		Labels:   map[string]string{managedLabel: "true"},
	})
	if err != nil {
		return fmt.Errorf("creating worker network %s: %w", netName, err)
	}
	return nil
}

// ConnectProxy attaches the proxy container to a worker's internal network so
// the worker can reach it (and route egress through it). Idempotent: a proxy
// already connected to the network is treated as success.
func (m *Manager) ConnectProxy(workerNet, proxyContainer string) error {
	ctx := context.Background()
	netName := WorkerNetworkName(workerNet)
	err := m.cli.NetworkConnect(ctx, netName, proxyContainer, &dockernetwork.EndpointSettings{})
	if err != nil && !isAlreadyConnected(err) {
		return fmt.Errorf("connecting proxy %q to %s: %w", proxyContainer, netName, err)
	}
	return nil
}

// DisconnectProxy detaches the proxy container from a worker's internal network.
// A proxy that is already disconnected (or a missing network) is not an error.
func (m *Manager) DisconnectProxy(workerNet, proxyContainer string) error {
	ctx := context.Background()
	netName := WorkerNetworkName(workerNet)
	err := m.cli.NetworkDisconnect(ctx, netName, proxyContainer, true)
	if err != nil && !isNotConnected(err) {
		return fmt.Errorf("disconnecting proxy %q from %s: %w", proxyContainer, netName, err)
	}
	return nil
}

// RemoveWorkerNetwork removes a worker's internal network. It first force-
// disconnects every endpoint still attached (the multi-homed proxy, plus any
// leftover worker container), since Docker refuses to remove a network with
// active endpoints. A missing network is not treated as an error. This lets
// callers tear the network down without needing a live proxy reference.
func (m *Manager) RemoveWorkerNetwork(name string) error {
	ctx := context.Background()
	netName := WorkerNetworkName(name)
	existing, err := m.findNetworkByName(ctx, netName)
	if err != nil {
		return err
	}
	if existing == nil {
		return nil
	}

	// Inspect to enumerate attached containers (the list endpoint omits them).
	if detail, err := m.cli.NetworkInspect(ctx, existing.ID, dockernetwork.InspectOptions{}); err == nil {
		for containerID := range detail.Containers {
			if derr := m.cli.NetworkDisconnect(ctx, existing.ID, containerID, true); derr != nil && !isNotConnected(derr) {
				return fmt.Errorf("disconnecting %s from %s: %w", containerID, netName, derr)
			}
		}
	}

	if err := m.cli.NetworkRemove(ctx, existing.ID); err != nil {
		return fmt.Errorf("removing worker network %s: %w", netName, err)
	}
	return nil
}

// WorkerNetworkExists reports whether the per-worker Internal network for the
// given worker name already exists.
func (m *Manager) WorkerNetworkExists(name string) (bool, error) {
	ctx := context.Background()
	net, err := m.findNetworkByName(ctx, WorkerNetworkName(name))
	if err != nil {
		return false, err
	}
	return net != nil, nil
}

// RemoveEgressNetwork removes the egress (proxy) network.
func (m *Manager) RemoveEgressNetwork() error {
	ctx := context.Background()
	existing, err := m.findNetworkByName(ctx, EgressNetworkName)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("%s does not exist", EgressNetworkName)
	}
	return m.cli.NetworkRemove(ctx, existing.ID)
}

// Status returns information about the egress network, or nil if it doesn't exist.
func (m *Manager) Status() (*NetworkInfo, error) {
	ctx := context.Background()
	net, err := m.findNetworkByName(ctx, EgressNetworkName)
	if err != nil {
		return nil, err
	}
	if net == nil {
		return nil, nil
	}
	info := &NetworkInfo{
		ID:       net.ID,
		Driver:   net.Driver,
		Internal: net.Internal,
	}
	if len(net.IPAM.Config) > 0 {
		info.Subnet = net.IPAM.Config[0].Subnet
	}
	return info, nil
}

// ListWorkerNetworks returns the names of all managed per-worker internal networks.
func (m *Manager) ListWorkerNetworks() ([]string, error) {
	ctx := context.Background()
	nets, err := m.cli.NetworkList(ctx, dockernetwork.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing networks: %w", err)
	}
	var names []string
	for i := range nets {
		if strings.HasPrefix(nets[i].Name, WorkerNetPrefix) {
			names = append(names, nets[i].Name)
		}
	}
	return names, nil
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

func isAlreadyConnected(err error) bool {
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "already exists in network") || strings.Contains(s, "already connected")
}

func isNotConnected(err error) bool {
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "is not connected to") ||
		strings.Contains(s, "not connected") ||
		strings.Contains(s, "no such network") ||
		strings.Contains(s, "not found")
}
