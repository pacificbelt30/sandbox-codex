package cmd

import (
	"fmt"

	"github.com/pacificbelt30/codex-dock/internal/network"
	"github.com/spf13/cobra"
)

var networkCreateNoInternet bool

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
		err = mgr.ApplyFirewall(network.EnsureOptions{NoInternet: networkCreateNoInternet})
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

func init() {
	rootCmd.AddCommand(networkCmd)
	networkCmd.AddCommand(networkCreateCmd)
	networkCmd.AddCommand(networkRmCmd)
	networkCmd.AddCommand(networkStatusCmd)
	networkCreateCmd.Flags().BoolVar(&networkCreateNoInternet, "no-internet", false, "Disable internet access inside dock-net")

	rootCmd.AddCommand(firewallCmd)
	firewallCmd.AddCommand(firewallCreateCmd)
	firewallCreateCmd.Flags().BoolVar(&networkCreateNoInternet, "no-internet", false, "Disable internet access inside dock-net")
}
