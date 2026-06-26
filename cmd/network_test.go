package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/pacificbelt30/codex-dock/internal/network"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func newFirewallCreateFlagSet() *cobra.Command {
	cmd := &cobra.Command{Use: "create"}
	cmd.Flags().String("proxy-container-url", "http://codex-auth-proxy:18080", "")
	cmd.Flags().StringArray("allow-host", nil, "")
	return cmd
}

func TestApplyFirewallConfigDefaults(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	prevURL := networkProxyContainerURL
	prevHosts := firewallAllowHosts
	t.Cleanup(func() {
		networkProxyContainerURL = prevURL
		firewallAllowHosts = prevHosts
	})

	networkProxyContainerURL = "http://codex-auth-proxy:18080"
	firewallAllowHosts = nil

	viper.Set("firewall.proxy_container_url", "http://proxy.internal:9000")
	viper.Set("firewall.allow_hosts", []string{"203.0.113.10:5000", "198.51.100.7:443"})

	applyFirewallConfigDefaults(newFirewallCreateFlagSet())

	if networkProxyContainerURL != "http://proxy.internal:9000" {
		t.Errorf("networkProxyContainerURL = %q; want config value", networkProxyContainerURL)
	}
	if len(firewallAllowHosts) != 2 || firewallAllowHosts[0] != "203.0.113.10:5000" {
		t.Errorf("firewallAllowHosts = %v; want config list", firewallAllowHosts)
	}
}

func TestApplyFirewallConfigDefaults_FlagPriority(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	prevURL := networkProxyContainerURL
	prevHosts := firewallAllowHosts
	t.Cleanup(func() {
		networkProxyContainerURL = prevURL
		firewallAllowHosts = prevHosts
	})

	networkProxyContainerURL = "http://flag.example:1234"
	firewallAllowHosts = []string{"192.0.2.1:8080"}

	viper.Set("firewall.proxy_container_url", "http://proxy.internal:9000")
	viper.Set("firewall.allow_hosts", []string{"203.0.113.10:5000"})

	cmd := newFirewallCreateFlagSet()
	if err := cmd.Flags().Set("proxy-container-url", "http://flag.example:1234"); err != nil {
		t.Fatalf("set proxy-container-url: %v", err)
	}
	if err := cmd.Flags().Set("allow-host", "192.0.2.1:8080"); err != nil {
		t.Fatalf("set allow-host: %v", err)
	}

	applyFirewallConfigDefaults(cmd)

	if networkProxyContainerURL != "http://flag.example:1234" {
		t.Errorf("networkProxyContainerURL = %q; want flag value", networkProxyContainerURL)
	}
	if len(firewallAllowHosts) != 1 || firewallAllowHosts[0] != "192.0.2.1:8080" {
		t.Errorf("firewallAllowHosts = %v; want flag value", firewallAllowHosts)
	}
}

func TestFormatFirewallRules(t *testing.T) {
	info := &network.FirewallInfo{
		ChainExists: true,
		Rules: []network.FirewallRule{
			{Action: "allow", Verdict: "RETURN", Destination: "172.17.0.1/32", Protocol: "tcp", Port: 18080, Comment: "codex-dock-allow-host"},
			{Action: "block", Verdict: "DROP", Destination: "10.0.0.0/8", Comment: "codex-dock-drop-private"},
			{Action: "allow", Verdict: "RETURN"},
		},
	}

	out := formatFirewallRules(info)

	for _, want := range []string{
		"ALLOW  172.17.0.1/32",
		"tcp/18080",
		"auth proxy / allowed host",
		"BLOCK  10.0.0.0/8",
		"private/link-local",
		"default: hand back to Docker rules",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("formatFirewallRules() missing %q\n%s", want, out)
		}
	}
}

func TestFormatFirewallRulesChainMissing(t *testing.T) {
	out := formatFirewallRules(&network.FirewallInfo{ChainExists: false})
	if !strings.Contains(out, "chain not installed") {
		t.Fatalf("formatFirewallRules() chain-missing hint absent\n%s", out)
	}
}

func TestFirewallVerdict(t *testing.T) {
	tests := []struct {
		name        string
		info        *network.FirewallInfo
		wantVerdict string
		wantHint    bool
	}{
		{
			name:        "non-linux",
			info:        &network.FirewallInfo{Supported: false},
			wantVerdict: "Unavailable (non-Linux host)",
			wantHint:    true,
		},
		{
			name:        "no iptables",
			info:        &network.FirewallInfo{Supported: true, IptablesFound: false},
			wantVerdict: "Unavailable (iptables not found)",
			wantHint:    true,
		},
		{
			name:        "active",
			info:        &network.FirewallInfo{Supported: true, IptablesFound: true, ChainExists: true, JumpRuleExists: true},
			wantVerdict: "Active",
			wantHint:    false,
		},
		{
			name:        "not active non-root",
			info:        &network.FirewallInfo{Supported: true, IptablesFound: true, Root: false},
			wantVerdict: "Not active",
			wantHint:    true,
		},
		{
			name:        "not active root",
			info:        &network.FirewallInfo{Supported: true, IptablesFound: true, Root: true},
			wantVerdict: "Not active",
			wantHint:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verdict, hint := firewallVerdict(tt.info)
			if verdict != tt.wantVerdict {
				t.Fatalf("firewallVerdict() verdict = %q, want %q", verdict, tt.wantVerdict)
			}
			if (hint != "") != tt.wantHint {
				t.Fatalf("firewallVerdict() hint = %q, wantHint = %v", hint, tt.wantHint)
			}
		})
	}
}

func TestConfirmCreateProxyNetworkYes(t *testing.T) {
	command := &cobra.Command{}
	command.SetIn(bytes.NewBufferString("y\n"))
	command.SetOut(&bytes.Buffer{})

	ok, err := confirmCreateProxyNetwork(command)
	if err != nil {
		t.Fatalf("confirmCreateProxyNetwork() error = %v", err)
	}
	if !ok {
		t.Fatalf("confirmCreateProxyNetwork() = %v, want true", ok)
	}
}

func TestConfirmCreateProxyNetworkNo(t *testing.T) {
	command := &cobra.Command{}
	command.SetIn(bytes.NewBufferString("n\n"))
	command.SetOut(&bytes.Buffer{})

	ok, err := confirmCreateProxyNetwork(command)
	if err != nil {
		t.Fatalf("confirmCreateProxyNetwork() error = %v", err)
	}
	if ok {
		t.Fatalf("confirmCreateProxyNetwork() = %v, want false", ok)
	}
}

func TestConfirmCreateNetworkYes(t *testing.T) {
	command := &cobra.Command{}
	command.SetIn(bytes.NewBufferString("yes\n"))
	command.SetOut(&bytes.Buffer{})

	ok, err := confirmCreateNetwork(command, "dock-net")
	if err != nil {
		t.Fatalf("confirmCreateNetwork() error = %v", err)
	}
	if !ok {
		t.Fatalf("confirmCreateNetwork() = %v, want true", ok)
	}
}

func TestConfirmCreateNetworkDefaultNo(t *testing.T) {
	command := &cobra.Command{}
	command.SetIn(bytes.NewBufferString("\n"))
	command.SetOut(&bytes.Buffer{})

	ok, err := confirmCreateNetwork(command, "dock-net")
	if err != nil {
		t.Fatalf("confirmCreateNetwork() error = %v", err)
	}
	if ok {
		t.Fatalf("confirmCreateNetwork() = %v, want false", ok)
	}
}
