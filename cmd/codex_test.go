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
	if !codexOptionPresent([]string{"-mother"}, "--model", "-m") {
		t.Fatal("short option with attached value not detected")
	}
	if codexOptionPresent([]string{"--", "--model", "other"}, "--model", "-m") {
		t.Fatal("option after -- must be treated as positional")
	}
}

func TestCodexConfigOverridePresent(t *testing.T) {
	for _, args := range [][]string{
		{"-c", "model=other"},
		{"--config", "model=other"},
		{"--config=model=other"},
		{"-cmodel=other"},
		{"-c", "model = other"},
	} {
		if !codexConfigOverridePresent(args, "model") {
			t.Errorf("model override not detected in %q", args)
		}
	}
	if codexConfigOverridePresent([]string{"-c", "model_provider=other"}, "model") {
		t.Fatal("unrelated config key detected as model")
	}
	if codexConfigOverridePresent([]string{"--", "-c", "model=other"}, "model") {
		t.Fatal("config override after -- must be treated as positional")
	}
	if codexConfigOverridePresent([]string{"-c", "--", "-c", "model=other"}, "model") {
		t.Fatal("missing config value must not consume the positional separator")
	}
}
