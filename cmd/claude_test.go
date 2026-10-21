package cmd

import (
	"reflect"
	"testing"

	"github.com/spf13/cobra"
)

func TestClaudeCommandForwardsNativeFlags(t *testing.T) {
	original := claudeCmd.RunE
	defer func() { claudeCmd.RunE = original; rootCmd.SetArgs(nil) }()
	var got []string
	claudeCmd.RunE = func(_ *cobra.Command, args []string) error { got = append([]string(nil), args...); return nil }
	want := []string{"-p", "prompt with spaces and $literal", "--model", "claude-sonnet-4-6", "--output-format", "stream-json"}
	rootCmd.SetArgs(append([]string{"claude"}, want...))
	if err := rootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("native arguments changed: %q", got)
	}
}
