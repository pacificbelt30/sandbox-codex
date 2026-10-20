package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/pacificbelt30/codex-dock/internal/kataguard"
	"github.com/spf13/cobra"
)

func newProfileCommand() *cobra.Command {
	var asJSON bool
	command := &cobra.Command{
		Use: "profile [standard|small|compact]", Short: "Show Kata worker resource profiles",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			profiles := kataguard.ResourceProfiles()
			if len(args) != 0 {
				profile, ok := kataguard.LookupResourceProfile(args[0])
				if !ok {
					return fmt.Errorf("unknown resource profile %q", args[0])
				}
				profiles = []kataguard.ResourceProfile{profile}
			}
			if asJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(profiles)
			}
			for _, profile := range profiles {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s: %d CPU, %.1f GiB guest RAM, %.1f GiB host limit, %d PIDs\n",
					profile.Name, profile.CPUCores, float64(profile.GuestMemoryBytes)/(1<<30), float64(profile.HostMemoryBytes)/(1<<30), profile.PIDLimit); err != nil {
					return err
				}
			}
			return nil
		},
	}
	command.Flags().BoolVar(&asJSON, "json", false, "Print a machine-readable resource profile")
	command.ValidArgsFunction = cobra.FixedCompletions([]cobra.Completion{"standard", "small", "compact"}, cobra.ShellCompDirectiveNoFileComp)
	return command
}

func init() { rootCmd.AddCommand(newProfileCommand()) }
