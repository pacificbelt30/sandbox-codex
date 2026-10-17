package network

import (
	dockernetwork "github.com/docker/docker/api/types/network"
	"testing"
)

func TestKataNetworkRejectsOrdinaryInternalBridge(t *testing.T) {
	n := dockernetwork.Inspect{Driver: "bridge", Internal: true, Options: map[string]string{}, Labels: map[string]string{managedLabel: "true", "codex-dock.isolation": "kata"}}
	if err := validateKataNetwork(n); err == nil {
		t.Fatal("ordinary Internal bridge accepted as host-isolated")
	}
	n.Options[isolatedIPv4] = "isolated"
	if err := validateKataNetwork(n); err != nil {
		t.Fatal(err)
	}
	n.EnableIPv6 = true
	if err := validateKataNetwork(n); err == nil {
		t.Fatal("IPv6 bridge without isolated mode accepted")
	}
	n.Options[isolatedIPv6] = "isolated"
	if err := validateKataNetwork(n); err != nil {
		t.Fatal(err)
	}
	n.Internal = false
	if err := validateKataNetwork(n); err == nil {
		t.Fatal("externally routed bridge accepted")
	}
}
