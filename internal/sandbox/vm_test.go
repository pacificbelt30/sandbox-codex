package sandbox

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/containerd/errdefs"
	"github.com/pacificbelt30/codex-dock/internal/kataguard"
)

type fixtureVMNode struct {
	fakeKataNode
	snapshot  kataguard.Inventory
	recovered []string
}

func (n *fixtureVMNode) Inventory(context.Context) (kataguard.Inventory, error) {
	return n.snapshot, nil
}
func (n *fixtureVMNode) RecoverOrphan(_ context.Context, id string, force bool) error {
	n.recovered = append(n.recovered, id)
	return nil
}
func TestVMManagementUsesWorkerLifecycleAndRetainsStoppedWorkers(t *testing.T) {
	f, m := newEngineFixture(t)
	node := &fixtureVMNode{snapshot: kataguard.Inventory{EngineAvailable: true}}
	m.guard = node
	id, err := m.Run(RunOptions{Name: "worker", ProjectDir: t.TempDir(), Detach: true})
	if err != nil {
		t.Fatal(err)
	}
	node.snapshot.VMs = []kataguard.VM{{ID: id, Name: "worker", Allocation: f.worker.Config.Labels[labelAllocation], ContainerID: id, ContainerPresent: true, State: "running"}}
	if err := m.VMStop(context.Background(), "worker", 1, false); err != nil {
		t.Fatal(err)
	}
	if f.worker == nil || f.worker.State.Running {
		t.Fatal("VM stop did not retain the stopped worker")
	}
	if err := m.VMStart(context.Background(), "worker"); err != nil {
		t.Fatal(err)
	}
	if !f.worker.State.Running {
		t.Fatal("VM start did not restart retained worker")
	}
	if err := m.VMRemove(context.Background(), "worker", true); err != nil {
		t.Fatal(err)
	}
	if f.worker != nil || len(node.recovered) != 0 {
		t.Fatal("ordinary worker bypassed lifecycle cleanup")
	}
}
func TestVMManagementNeverRecoversWhenDockerIsUnavailable(t *testing.T) {
	_, m := newEngineFixture(t)
	node := &fixtureVMNode{}
	m.guard = node
	if err := m.VMRemove(context.Background(), "anything", true); err == nil {
		t.Fatal("removal allowed during Docker failure")
	}
	if _, err := m.VMPrune(context.Background()); err == nil {
		t.Fatal("pruning allowed during Docker failure")
	}
	if len(node.recovered) != 0 {
		t.Fatal("unknown state was treated as an orphan")
	}
}
func TestVMPruneRetainsLiveOrphansAndStoppedWorkers(t *testing.T) {
	_, m := newEngineFixture(t)
	node := &fixtureVMNode{snapshot: kataguard.Inventory{EngineAvailable: true, VMs: []kataguard.VM{{Allocation: "empty", Orphan: true}, {Allocation: "live", Orphan: true, Populated: true}, {Allocation: "retained", ContainerPresent: true, State: "exited"}}}}
	m.guard = node
	removed, err := m.VMPrune(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0] != "empty" || len(node.recovered) != 1 {
		t.Fatalf("unexpected prune: %v %v", removed, node.recovered)
	}
}
func TestVMRemovalResumesPendingCleanupAfterBothInventoriesLoseWorker(t *testing.T) {
	f, m := newEngineFixture(t)
	node := &fixtureVMNode{snapshot: kataguard.Inventory{EngineAvailable: true}}
	m.guard = node
	id, err := m.Run(RunOptions{Name: "worker", ProjectDir: t.TempDir(), Detach: true})
	if err != nil {
		t.Fatal(err)
	}
	f.networkRemoveError = true
	if err = m.Remove(id, true); err == nil {
		t.Fatal("fault injection did not fail")
	}
	f.networkRemoveError = false
	if err = m.VMRemove(context.Background(), "worker", true); err != nil {
		t.Fatal(err)
	}
	if _, err = m.readCleanup(id); !os.IsNotExist(err) {
		t.Fatal("pending cleanup was not completed:", err)
	}
}

func TestAmbiguousVMRemovalNeverFallsBackToPendingCleanup(t *testing.T) {
	f, m := newEngineFixture(t)
	node := &fixtureVMNode{snapshot: kataguard.Inventory{EngineAvailable: true}}
	m.guard = node
	id, err := m.Run(RunOptions{Name: "worker", ProjectDir: t.TempDir(), Detach: true})
	if err != nil {
		t.Fatal(err)
	}
	f.networkRemoveError = true
	if err = m.Remove(id, true); err == nil {
		t.Fatal("fault injection did not fail")
	}
	f.networkRemoveError = false
	node.snapshot.VMs = []kataguard.VM{{ID: "first", Name: "worker"}, {ID: "second", Name: "worker"}}
	if err := m.VMRemove(context.Background(), "worker", true); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatal("ambiguous reference was accepted:", err)
	}
	if _, err := m.readCleanup(id); err != nil {
		t.Fatal("ambiguous request consumed the cleanup record:", err)
	}
}

func TestMissingWorkerAfterRemovalHasNotFoundCategory(t *testing.T) {
	_, m := newEngineFixture(t)
	id, err := m.Run(RunOptions{Name: "worker", ProjectDir: t.TempDir(), Detach: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Remove(id, true); err != nil {
		t.Fatal(err)
	}
	if err := m.Remove(id, true); !errdefs.IsNotFound(err) {
		t.Fatalf("repeated removal lost not-found category: %v", err)
	}
	if _, err := m.resolveID("worker"); !errdefs.IsNotFound(err) {
		t.Fatalf("missing worker lost not-found category: %v", err)
	}
}
