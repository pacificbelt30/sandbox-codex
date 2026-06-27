package cmd

import (
	"fmt"

	"github.com/pacificbelt30/codex-dock/internal/network"
	"github.com/spf13/cobra"
)

var networkCmd = &cobra.Command{
	Use:   "network",
	Short: "Manage codex-dock Docker networks",
	Long: `Manage codex-dock's Docker networks.

Isolation is enforced entirely by Docker network primitives (no iptables/sudo):
  - the egress network (dock-net-proxy) gives the auth proxy internet access;
  - each worker gets its own Internal network shared only with the proxy, so
    workers cannot reach each other, the host, or the internet directly — all
    egress flows through the proxy/router.`,
}

var networkCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create the egress (proxy) network",
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := network.NewManager()
		if err != nil {
			return err
		}
		if err := mgr.EnsureEgressNetwork(); err != nil {
			return fmt.Errorf("creating %s: %w", network.EgressNetworkName, err)
		}
		fmt.Printf("%s created.\n", network.EgressNetworkName)
		return nil
	},
}

var networkRmCmd = &cobra.Command{
	Use:   "rm",
	Short: "Remove the egress (proxy) network",
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := network.NewManager()
		if err != nil {
			return err
		}
		if err := mgr.RemoveEgressNetwork(); err != nil {
			return fmt.Errorf("removing %s: %w", network.EgressNetworkName, err)
		}
		fmt.Printf("%s removed.\n", network.EgressNetworkName)
		return nil
	},
}

var networkStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show network status",
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
			fmt.Printf("%s: not created\n", network.EgressNetworkName)
		} else {
			fmt.Printf("Egress network:  %s\n", network.EgressNetworkName)
			fmt.Printf("  ID:            %s\n", info.ID[:12])
			fmt.Printf("  Driver:        %s\n", info.Driver)
			fmt.Printf("  Internal:      %v\n", info.Internal)
			fmt.Printf("  Subnet:        %s\n", info.Subnet)
		}

		workerNets, err := mgr.ListWorkerNetworks()
		if err != nil {
			return fmt.Errorf("listing worker networks: %w", err)
		}
		fmt.Printf("Worker networks: %d (Internal, one per worker)\n", len(workerNets))
		for _, n := range workerNets {
			fmt.Printf("  - %s\n", n)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(networkCmd)
	networkCmd.AddCommand(networkCreateCmd)
	networkCmd.AddCommand(networkRmCmd)
	networkCmd.AddCommand(networkStatusCmd)
}
