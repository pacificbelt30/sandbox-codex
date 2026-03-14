package cmd

import (
	"fmt"

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
	firewallCreateCmd.Flags().StringVar(&networkProxyContainerURL, "proxy-container-url", "http://10.200.0.1:18080", "Auth proxy URL reachable from worker containers")
}
