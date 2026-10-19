package workeregress

import (
	"github.com/docker/docker/api/types/container"
	dn "github.com/docker/docker/api/types/network"
	"net/netip"
	"testing"
)

func TestInspectResolvesOnlyAdvertisedPinnedProxyIdentity(t *testing.T) {
	worker := container.InspectResponse{ContainerJSONBase: &container.ContainerJSONBase{HostConfig: &container.HostConfig{ExtraHosts: []string{"auth:10.253.1.2", "egress:10.253.1.3", "unadvertised:10.253.1.9"}}}, Config: &container.Config{Labels: map[string]string{"codex-dock.proxy-hosts": `["auth","egress"]`}}}
	name, err := proxyReference(worker, "10.253.1.3")
	if err != nil || name != "egress" {
		t.Fatalf("pinned IP mapping %s %v", name, err)
	}
	if _, err := proxyReference(worker, "10.253.1.9"); err == nil {
		t.Fatal("unadvertised mapping accepted")
	}
	if _, err := proxyReference(worker, "10.253.9.9"); err == nil {
		t.Fatal("unknown proxy IP accepted")
	}
	if name, err := proxyReference(worker, "legacy-egress"); err != nil || name != "legacy-egress" {
		t.Fatal("hostname lookup changed")
	}
	worker.HostConfig.ExtraHosts = append(worker.HostConfig.ExtraHosts, "auth:10.253.1.3")
	if _, err := proxyReference(worker, "10.253.1.3"); err == nil {
		t.Fatal("ambiguous proxy mapping accepted")
	}
}
func TestInspectRejectsProxyAddressOrNetworkDrift(t *testing.T) {
	worker := container.InspectResponse{NetworkSettings: &container.NetworkSettings{Networks: map[string]*dn.EndpointSettings{"front": {NetworkID: "front-id", IPAddress: "10.253.1.4"}}}}
	proxy := container.InspectResponse{NetworkSettings: &container.NetworkSettings{Networks: map[string]*dn.EndpointSettings{"front": {NetworkID: "front-id", IPAddress: "10.253.1.3"}}}}
	address := netip.MustParseAddr("10.253.1.3")
	if !proxySharesAddress(worker, proxy, address) {
		t.Fatal("valid shared address rejected")
	}
	proxy.NetworkSettings.Networks["front"].NetworkID = "other-network"
	if proxySharesAddress(worker, proxy, address) {
		t.Fatal("different network accepted")
	}
	proxy.NetworkSettings.Networks["front"].NetworkID = "front-id"
	proxy.NetworkSettings.Networks["front"].IPAddress = "10.253.1.8"
	if proxySharesAddress(worker, proxy, address) {
		t.Fatal("changed proxy address accepted")
	}
}
