package cmd

import (
	"reflect"
	"testing"

	"github.com/spf13/cobra"
)

func TestCodexCommandForwardsFlagsAndPrompt(t *testing.T) {
	original := codexCmd.RunE
	defer func() { codexCmd.RunE = original }()
	var got []string
	codexCmd.RunE = func(_ *cobra.Command, args []string) error {
		got = append([]string(nil), args...)
		return nil
	}
	want := []string{"--model", "test-model", "exec", "prompt with spaces", "--json"}
	rootCmd.SetArgs(append([]string{"codex"}, want...))
	defer rootCmd.SetArgs(nil)
	if err := rootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("forwarded = %q; want %q", got, want)
	}
}

func TestCodexOptionPresent(t *testing.T) {
	if !codexOptionPresent([]string{"exec", "--model=other", "prompt"}, "--model", "-m") {
		t.Fatal("long option with value not detected")
	}
	if !codexOptionPresent([]string{"-m", "other"}, "--model", "-m") {
		t.Fatal("short option not detected")
	}
	if codexOptionPresent([]string{"--", "--model", "other"}, "--model", "-m") {
		t.Fatal("option after -- must be treated as positional")
	}
}
