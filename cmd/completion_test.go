package cmd

import (
	"slices"
	"testing"

	"github.com/pacificbelt30/codex-dock/internal/kataguard"
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

func TestVMCompletionUsesInventoryAndAction(t *testing.T) {
	for _, action := range []string{"inspect", "start", "stop", "rm"} {
		command, _, err := newVMCommand().Find([]string{action})
		if err != nil || command.ValidArgsFunction == nil {
			t.Fatalf("vm %s has no positional completion: %v", action, err)
		}
		if action == "inspect" {
			values, directive := command.ValidArgsFunction(command, []string{"existing"}, "")
			if len(values) != 0 || directive != cobra.ShellCompDirectiveNoFileComp {
				t.Fatalf("vm inspect offered a second argument: %q, %v", values, directive)
			}
		}
	}
	inventory := kataguard.Inventory{EngineAvailable: true, VMs: []kataguard.VM{
		{ID: "running-id", Name: "running", ContainerPresent: true, EngineState: "running"},
		{ID: "stopped-id", Name: "stopped", ContainerPresent: true, EngineState: "exited"},
		{ID: "orphan-id", Allocation: "orphan-id", Orphan: true, State: "orphan-running"},
	}}
	for _, tc := range []struct {
		action string
		want   []cobra.Completion
	}{
		{"inspect", []cobra.Completion{"orphan-id", "running", "stopped"}},
		{"start", []cobra.Completion{"stopped"}},
		{"stop", []cobra.Completion{"orphan-id", "running"}},
		{"rm", []cobra.Completion{"orphan-id", "running", "stopped"}},
	} {
		got := vmCompletionNames(inventory, nil, tc.action)
		if !slices.Equal(got, tc.want) {
			t.Errorf("vm %s completion = %q; want %q", tc.action, got, tc.want)
		}
	}
	if got := vmCompletionNames(inventory, []string{"running"}, "rm"); !slices.Equal(got, []cobra.Completion{"orphan-id", "stopped"}) {
		t.Errorf("repeated VM candidate = %q", got)
	}
	inventory.EngineAvailable = false
	if got := vmCompletionNames(inventory, nil, "stop"); len(got) != 0 {
		t.Errorf("unavailable engine offered mutable VM candidates: %q", got)
	}
	if got := vmCompletionNames(inventory, nil, "inspect"); len(got) != 3 {
		t.Errorf("unavailable engine hid inspect candidates: %q", got)
	}
}

func TestVMCompletionAvoidsAmbiguousNames(t *testing.T) {
	inventory := kataguard.Inventory{EngineAvailable: true, VMs: []kataguard.VM{
		{ID: "first-id", Name: "shared", ContainerPresent: true},
		{ID: "second-id", Name: "shared", ContainerPresent: true},
		{ID: "third-id", Name: "second-id", ContainerPresent: true},
	}}
	if got := vmCompletionNames(inventory, nil, "inspect"); !slices.Equal(got, []cobra.Completion{"first-id", "third-id"}) {
		t.Errorf("ambiguous VM completion = %q", got)
	}
}
