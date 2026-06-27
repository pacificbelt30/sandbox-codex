package cmd

import (
	"testing"
)

// TestNetworkCommandStructure verifies the network command keeps its
// create/rm/status subcommands and that the firewall group is gone.
func TestNetworkCommandStructure(t *testing.T) {
	var sub []string
	for _, c := range networkCmd.Commands() {
		sub = append(sub, c.Name())
	}
	for _, want := range []string{"create", "rm", "status"} {
		if !containsString(sub, want) {
			t.Errorf("network command missing %q subcommand; have %v", want, sub)
		}
	}
	for _, c := range rootCmd.Commands() {
		if c.Name() == "firewall" {
			t.Errorf("firewall command group should have been removed")
		}
	}
}

func containsString(s []string, want string) bool {
	for _, v := range s {
		if v == want {
			return true
		}
	}
	return false
}
