package workeregress

import (
	"context"
	"encoding/json"
	"github.com/docker/docker/api/types/container"
	dn "github.com/docker/docker/api/types/network"
	"github.com/pacificbelt30/codex-dock/internal/networkpolicy"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectParsesStrictPolicyWithoutChangingIdentity(t *testing.T) {
	rules, err := ParseProxyRules([]string{"proxy", "serve", "--role", "egress", "--block-private", "--forward-default-deny", "--forward-allow-domain", "=example.com"})
	want := networkpolicy.Rules{DefaultDeny: true, BlockPrivate: true, AllowDomains: []string{"=example.com"}}
	if err != nil || ID(rules) != ID(want) {
		t.Fatalf("running policy differed from its configuration: %+v %v", rules, err)
	}
	if _, err := ParseProxyRules([]string{"--role", "egress", "--forward-default-deny=maybe"}); err == nil {
		t.Fatal("invalid boolean accepted")
	}
}

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

func TestInspectDistinguishesPendingQuarantineFromCompletedStop(t *testing.T) {
	for _, test := range []struct {
		name    string
		state   *container.State
		pending bool
	}{
		{"exited", &container.State{Status: "exited"}, false},
		{"created", &container.State{Status: "created"}, false},
		{"running", &container.State{Running: true, Status: "running"}, true},
		{"paused", &container.State{Paused: true, Status: "paused"}, true},
		{"unknown", nil, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir, err := os.MkdirTemp("", "cxd-policy-")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(dir)
			socket := filepath.Join(dir, "node.sock")
			ln, err := net.Listen("unix", socket)
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !strings.HasSuffix(r.URL.Path, "/containers/worker/json") {
					t.Errorf("unexpected lookup: %s", r.URL.Path)
					w.WriteHeader(500)
					return
				}
				_ = json.NewEncoder(w).Encode(container.InspectResponse{ContainerJSONBase: &container.ContainerJSONBase{ID: "worker", State: test.state}, Config: &container.Config{Labels: map[string]string{managedLabel: "true", "codex-dock.quarantined": "true"}}})
			}))
			_ = server.Listener.Close()
			server.Listener = ln
			server.Start()
			defer server.Close()
			snapshot, err := Inspect(context.Background(), socket, "worker")
			if err != nil || !snapshot.Quarantined || snapshot.QuarantinePending != test.pending {
				t.Fatalf("quarantine status misrepresented: %+v %v", snapshot, err)
			}
		})
	}
}
