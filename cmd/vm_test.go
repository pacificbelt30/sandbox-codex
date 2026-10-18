package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/pacificbelt30/codex-dock/internal/kataguard"
	"github.com/spf13/cobra"
)

func TestVMCommandsExposeExpectedManagementSurface(t *testing.T) {
	for _, args := range [][]string{{"ps", "--all", "--json"}, {"inspect", "worker"}, {"start", "worker"}, {"stop", "--force", "worker"}, {"rm", "--force", "worker"}, {"prune"}} {
		root := newVMCommand()
		cmd, _, err := root.Find(args)
		if err != nil || cmd == root {
			t.Fatalf("missing VM command %v: %v", args, err)
		}
	}
}

func TestTopLevelStartMatchesWorkerManagement(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"start", "worker"})
	if err != nil || cmd != startCmd {
		t.Fatalf("top-level start command missing: %v", err)
	}
	if err := startCmd.Args(startCmd, nil); err == nil {
		t.Fatal("start should require a worker name or ID")
	}
}
func TestVMTableKeepsUnknownAndOrphanStatesVisible(t *testing.T) {
	rows := []kataguard.VM{{ID: "run", State: "running"}, {ID: "stopped", State: "exited"}, {ID: "unknown", State: "unknown"}, {ID: "orphan", State: "orphan-empty", Orphan: true}}
	visible := visibleVMs(rows, false)
	if len(visible) != 3 {
		t.Fatalf("unexpected visible rows: %+v", visible)
	}
	out := new(bytes.Buffer)
	cmd := &cobra.Command{}
	cmd.SetOut(out)
	cmd.SetErr(out)
	if err := printVMs(cmd, kataguard.Inventory{VMs: visible, Problems: []string{"Docker unavailable"}}); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"unknown", "orphan-empty", "Docker unavailable"} {
		if !strings.Contains(out.String(), text) {
			t.Fatal("missing diagnostic:", text)
		}
	}
	if len(visibleVMs(rows, true)) != 4 {
		t.Fatal("--all omitted a stopped worker")
	}
}
