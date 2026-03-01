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
				{Subnet: "192.168.200.0/24"},
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
