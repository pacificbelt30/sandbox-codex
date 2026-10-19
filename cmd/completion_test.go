package cmd

import (
	"slices"
	"testing"

	"github.com/pacificbelt30/codex-dock/internal/sandbox"
	"github.com/spf13/cobra"
)

func TestCompletionChoices(t *testing.T) {
	values, directive := defaultCmd.ValidArgsFunction(defaultCmd, nil, "c")
	if directive != cobra.ShellCompDirectiveNoFileComp || !slices.Contains(values, "codex") {
		t.Fatalf("default completion = %q, directive %v", values, directive)
	}
	if values, _ := defaultCmd.ValidArgsFunction(defaultCmd, []string{"codex"}, "c"); len(values) != 0 {
		t.Fatalf("default accepted a second value: %q", values)
	}
	for _, tc := range []struct {
		cmd  *cobra.Command
		flag string
		want string
	}{
		{runCmd, "agent", "codex"},
		{runCmd, "runtime", "kata"},
		{runCmd, "approval-mode", "auto-edit"},
		{runCmd, "shell-type", "zsh"},
		{runCmd, "template", "plain"},
		{runCmd, "image", "codex-dock:latest"},
		{buildCmd, "template", "plain"},
	} {
		fn, ok := tc.cmd.GetFlagCompletionFunc(tc.flag)
		if !ok {
			t.Fatalf("%s --%s has no value completion", tc.cmd.Name(), tc.flag)
		}
		values, directive := fn(tc.cmd, nil, "")
		if directive != cobra.ShellCompDirectiveNoFileComp || !slices.Contains(values, tc.want) {
			t.Fatalf("%s --%s completion = %q, directive %v; want %q", tc.cmd.Name(), tc.flag, values, directive, tc.want)
		}
	}
}

func TestWorkerCompletionNames(t *testing.T) {
	workers := []sandbox.Worker{
		{Name: "zeta", Status: "running"},
		{Name: "alpha", Status: "running"},
		{Name: "stopped", Status: "exited"},
	}
	got := workerCompletionNames(workers, []string{"zeta"}, func(w sandbox.Worker) bool { return w.Status == "running" })
	if !slices.Equal(got, []string{"alpha"}) {
		t.Fatalf("running worker completion = %q", got)
	}
	got = workerCompletionNames(workers, nil, func(w sandbox.Worker) bool { return w.Status != "running" })
	if !slices.Equal(got, []string{"stopped"}) {
		t.Fatalf("stopped worker completion = %q", got)
	}
}
