package cmd

import (
	"slices"
	"testing"

	"github.com/spf13/cobra"
)

func TestExecDockerStyleSeparatorIsNotGuestCommand(t *testing.T) {
	for _, input := range [][]string{{"-i", "worker", "--", "sh", "-c", "echo ok"}, {"worker", "sh", "-c", "echo ok"}} {
		cmd := &cobra.Command{Use: "exec", Args: cobra.MinimumNArgs(2)}
		cmd.Flags().SetInterspersed(false)
		cmd.Flags().BoolP("interactive", "i", false, "")
		cmd.SetArgs(input)
		cmd.RunE = func(_ *cobra.Command, args []string) error {
			if args[0] != "worker" || !slices.Equal(execArguments(args), []string{"sh", "-c", "echo ok"}) {
				t.Fatalf("guest argv = %q", args)
			}
			return nil
		}
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
	}
}
