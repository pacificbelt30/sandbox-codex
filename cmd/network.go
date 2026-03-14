package cmd

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/pacificbelt30/codex-dock/internal/network"
	"github.com/spf13/cobra"
)

var (
	networkCreateNoInternet  bool
	networkProxyContainerURL string
)

var networkCmd = &cobra.Command{
	Use:   "network",
	Short: "Manage dock-net Docker network",
}

var networkCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create dock-net",
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := network.NewManager()
		if err != nil {
			return err
		}
		if err := mgr.EnsureNetwork(network.EnsureOptions{NoInternet: networkCreateNoInternet}); err != nil {
			return fmt.Errorf("creating dock-net: %w", err)
		}
		fmt.Println("dock-net created.")
		return nil
	},
}

var networkRmCmd = &cobra.Command{
	Use:   "rm",
	Short: "Remove dock-net",
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := network.NewManager()
		if err != nil {
			return err
		}
		if err := mgr.RemoveNetwork(); err != nil {
			return fmt.Errorf("removing dock-net: %w", err)
		}
		fmt.Println("dock-net removed.")
		return nil
	},
}

var networkStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show dock-net status",
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := network.NewManager()
		if err != nil {
			return err
		}
		info, err := mgr.Status()
		if err != nil {
			return fmt.Errorf("getting network status: %w", err)
		}
		if info == nil {
			fmt.Println("dock-net: not created")
			return nil
		}
		fmt.Printf("dock-net ID:     %s\n", info.ID[:12])
		fmt.Printf("Driver:          %s\n", info.Driver)
		fmt.Printf("ICC disabled:    %v\n", info.ICCDisabled)
		fmt.Printf("IP Masquerade:   %v\n", info.IPMasquerade)
		fmt.Printf("Subnet:          %s\n", info.Subnet)
		return nil
	},
}

var firewallCmd = &cobra.Command{
	Use:   "firewall",
	Short: "Manage dock-net firewall rules",
}

var firewallCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create dock-net firewall rules",
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := network.NewManager()
		if err != nil {
			return err
		}
		ensureOpts := network.EnsureOptions{NoInternet: networkCreateNoInternet}

		networkInfo, err := mgr.Status()
		if err != nil {
			return fmt.Errorf("checking %s: %w", network.NetworkName, err)
		}
		if networkInfo == nil {
			fmt.Printf("Warning: %s network is not present. Firewall setup requires it.\n", network.NetworkName)
			createNetwork, err := confirmCreateNetwork(cmd, network.NetworkName)
			if err != nil {
				return err
			}
			if createNetwork {
				if err := mgr.EnsureNetwork(ensureOpts); err != nil {
					return fmt.Errorf("creating %s: %w", network.NetworkName, err)
				}
				fmt.Printf("%s network created.\n", network.NetworkName)
			} else {
				return fmt.Errorf("%s network is required for firewall setup", network.NetworkName)
			}
		}

		proxyNetworkExists, err := mgr.ProxyNetworkExists()
		if err != nil {
			return fmt.Errorf("checking %s: %w", network.ProxyNetworkName, err)
		}
		if !proxyNetworkExists {
			fmt.Printf("Warning: %s network is not present. Proxy NIC-level firewall allow rules will not be installed.\n", network.ProxyNetworkName)
			createNetwork, err := confirmCreateNetwork(cmd, network.ProxyNetworkName)
			if err != nil {
				return err
			}
			if createNetwork {
				if err := mgr.EnsureProxyNetwork(); err != nil {
					return fmt.Errorf("creating %s: %w", network.ProxyNetworkName, err)
				}
				fmt.Printf("%s network created.\n", network.ProxyNetworkName)
			} else {
				fmt.Printf("Skipping %s creation; only CODEX-DOCK rules will be applied.\n", network.ProxyNetworkName)
			}
		}

		if port, ok := allowedHostPort(networkProxyContainerURL); ok {
			ensureOpts.AllowHostTCPPorts = []int{port}
		}
		if endpoint, ok := network.AllowHostEndpoint(networkProxyContainerURL); ok {
			ensureOpts.AllowTCPDestinations = []network.HostEndpoint{endpoint}
		}
		err = mgr.ApplyFirewall(ensureOpts)
		if err != nil {
			if network.IsFirewallWarning(err) {
				fmt.Printf("Warning: dock-net firewall rules were not applied: %v\n", err)
				return nil
			}
			return fmt.Errorf("creating dock-net firewall rules: %w", err)
		}
		fmt.Println("dock-net firewall rules created.")
		return nil
	},
}

var firewallRmCmd = &cobra.Command{
	Use:   "rm",
	Short: "Remove dock-net firewall rules",
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := network.NewManager()
		if err != nil {
			return err
		}
		err = mgr.RemoveFirewall()
		if err != nil {
			if network.IsFirewallWarning(err) {
				fmt.Printf("Warning: dock-net firewall rules were not removed: %v\n", err)
				return nil
			}
			return fmt.Errorf("removing dock-net firewall rules: %w", err)
		}
		fmt.Println("dock-net firewall rules removed.")
		return nil
	},
}

var firewallStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show dock-net firewall status",
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := network.NewManager()
		if err != nil {
			return err
		}
		info, err := mgr.FirewallStatus()
		if err != nil {
			return fmt.Errorf("getting dock-net firewall status: %w", err)
		}

		fmt.Printf("Supported (Linux): %v\n", info.Supported)
		fmt.Printf("Running as root:   %v\n", info.Root)
		fmt.Printf("iptables found:    %v\n", info.IptablesFound)
		fmt.Printf("Chain exists:      %v\n", info.ChainExists)
		fmt.Printf("Jump rule exists:  %v\n", info.JumpRuleExists)
		if info.DockerUserDefaultPolicy != "" {
			fmt.Printf("DOCKER-USER policy: %s\n", info.DockerUserDefaultPolicy)
		} else {
			fmt.Println("DOCKER-USER policy: (unknown)")
		}
		if info.ManagedChainFinalVerdict != "" {
			fmt.Printf("CODEX-DOCK final jump: %s\n", info.ManagedChainFinalVerdict)
		} else {
			fmt.Println("CODEX-DOCK final jump: (unknown)")
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(networkCmd)
	networkCmd.AddCommand(networkCreateCmd)
	networkCmd.AddCommand(networkRmCmd)
	networkCmd.AddCommand(networkStatusCmd)
	networkCreateCmd.Flags().BoolVar(&networkCreateNoInternet, "no-internet", false, "Disable internet access inside dock-net")

	rootCmd.AddCommand(firewallCmd)
	firewallCmd.AddCommand(firewallCreateCmd)
	firewallCmd.AddCommand(firewallRmCmd)
	firewallCmd.AddCommand(firewallStatusCmd)
	firewallCreateCmd.Flags().BoolVar(&networkCreateNoInternet, "no-internet", false, "Disable internet access inside dock-net")
	firewallCreateCmd.Flags().StringVar(&networkProxyContainerURL, "proxy-container-url", "http://codex-auth-proxy:18080", "Auth proxy URL reachable from worker containers")
}

func confirmCreateProxyNetwork(cmd *cobra.Command) (bool, error) {
	return confirmCreateNetwork(cmd, network.ProxyNetworkName)
}

func confirmCreateNetwork(cmd *cobra.Command, networkName string) (bool, error) {
	prompt := fmt.Sprintf("Create %s now? [y/N]: ", networkName)
	if _, err := fmt.Fprint(cmd.OutOrStdout(), prompt); err != nil {
		return false, fmt.Errorf("prompting for %s creation: %w", networkName, err)
	}

	reader := bufio.NewReader(cmd.InOrStdin())
	line, err := reader.ReadString('\n')
	if err != nil && strings.TrimSpace(line) == "" {
		return false, fmt.Errorf("reading confirmation input: %w", err)
	}

	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}
