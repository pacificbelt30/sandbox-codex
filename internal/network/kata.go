package network

import (
	"context"
	"fmt"
	"net/netip"
	"sort"
	"strings"

	"github.com/containerd/errdefs"
	dockernetwork "github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
)

const isolatedIPv4 = "com.docker.network.bridge.gateway_mode_ipv4"
const isolatedIPv6 = "com.docker.network.bridge.gateway_mode_ipv6"

// NewManagerWithClient shares the selected daemon with the sandbox manager.
func NewManagerWithClient(cli *client.Client) *Manager { return &Manager{cli: cli} }

// CreateKataWorkerNetwork never adopts a pre-existing network: an old name is
// not proof that it is empty or belongs to this new run.
func (m *Manager) CreateKataWorkerNetwork(ctx context.Context, worker string) (string, error) {
	name := WorkerNetworkName(worker)
	if _, err := m.cli.NetworkInspect(ctx, name, dockernetwork.InspectOptions{}); err == nil {
		return "", fmt.Errorf("network %s already exists; remove the old worker/network before reusing its name", name)
	} else if !errdefs.IsNotFound(err) {
		return "", err
	}
	f := false
	r, err := m.cli.NetworkCreate(ctx, name, dockernetwork.CreateOptions{
		Driver: "bridge", Internal: true, EnableIPv6: &f,
		Options: map[string]string{isolatedIPv4: "isolated", isolatedIPv6: "isolated"},
		Labels:  map[string]string{managedLabel: "true", "codex-dock.isolation": "kata"},
	})
	if err != nil {
		return "", fmt.Errorf("creating Kata network (requires Docker isolated gateway support): %w", err)
	}
	return r.ID, nil
}

// KataProxyHosts validates the entire front network and returns its current
// proxy addresses for /etc/hosts. Exactly one worker and its selected proxies
// may be attached. With --no-internet, the egress proxy is not in proxies.
func (m *Manager) KataProxyHosts(ctx context.Context, networkID, workerID string, proxies []string) ([]string, error) {
	n, err := m.cli.NetworkInspect(ctx, networkID, dockernetwork.InspectOptions{})
	if err != nil {
		return nil, err
	}
	if err := validateKataNetwork(n); err != nil {
		return nil, err
	}
	allowed := map[string]bool{}
	if workerID != "" {
		allowed[workerID] = true
	}
	hosts := make([]string, 0, len(proxies))
	for _, host := range proxies {
		p, err := m.cli.ContainerInspect(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("inspecting proxy %s: %w", host, err)
		}
		if p.State == nil || !p.State.Running {
			return nil, fmt.Errorf("proxy %s is not running", host)
		}
		if p.ID == workerID || allowed[p.ID] {
			return nil, fmt.Errorf("proxy %s has a conflicting identity", host)
		}
		e, ok := n.Containers[p.ID]
		if !ok {
			return nil, fmt.Errorf("proxy %s is not attached to worker network", host)
		}
		prefix, err := netip.ParsePrefix(e.IPv4Address)
		if err != nil || !prefix.Addr().Is4() || prefix.Addr().IsUnspecified() || prefix.Addr().IsLoopback() {
			return nil, fmt.Errorf("proxy %s has no usable front IPv4", host)
		}
		if host == "" || strings.ContainsAny(host, ":/ \r\n\t\x00") {
			return nil, fmt.Errorf("invalid proxy hostname")
		}
		hosts = append(hosts, host+":"+prefix.Addr().String())
		allowed[p.ID] = true
	}
	for id := range n.Containers {
		if !allowed[id] {
			return nil, fmt.Errorf("unexpected peer on Kata worker network: %s", id)
		}
	}
	sort.Strings(hosts)
	return hosts, nil
}

func validateKataNetwork(n dockernetwork.Inspect) error {
	if n.Driver != "bridge" || !n.Internal || n.Options[isolatedIPv4] != "isolated" || (n.EnableIPv6 && n.Options[isolatedIPv6] != "isolated") || n.Labels[managedLabel] != "true" || n.Labels["codex-dock.isolation"] != "kata" {
		return fmt.Errorf("Kata requires a managed internal bridge with isolated gateway mode")
	}
	return nil
}

// RemoveKataWorkerNetwork cleans up only the exact network ID created by a
// failed launch. Never disconnect an unexpected peer to make deletion succeed.
func (m *Manager) RemoveKataWorkerNetwork(ctx context.Context, id string, proxies []string) error {
	n, err := m.cli.NetworkInspect(ctx, id, dockernetwork.InspectOptions{})
	if errdefs.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := validateKataNetwork(n); err != nil {
		return err
	}
	allowed := map[string]bool{}
	for _, name := range proxies {
		p, err := m.cli.ContainerInspect(ctx, name)
		if errdefs.IsNotFound(err) {
			continue
		}
		if err != nil {
			return err
		}
		allowed[p.ID] = true
	}
	for peer := range n.Containers {
		if !allowed[peer] {
			return fmt.Errorf("refusing to disconnect unexpected peer %s during cleanup", peer)
		}
	}
	for peer := range n.Containers {
		if err := m.cli.NetworkDisconnect(ctx, id, peer, true); err != nil {
			return err
		}
	}
	return m.cli.NetworkRemove(ctx, id)
}
