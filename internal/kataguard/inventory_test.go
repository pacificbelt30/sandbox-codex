package kataguard

import (
	"strings"
	"testing"
	"time"
)

func TestInventoryReconcilesIndependentcontainerdAndHostState(t *testing.T) {
	allocation := strings.Repeat("a", 32)
	worker := strings.Repeat("b", 64)
	for _, tc := range []struct {
		name                                               string
		available, present, allocated, populated, verified bool
		docker, want                                       string
	}{
		{"running", true, true, true, true, true, "running", "running"},
		{"paused", true, true, true, true, true, "paused", "paused"},
		{"stopped", true, true, false, false, false, "exited", "exited"},
		{"missing-vm", true, true, false, false, false, "running", "inconsistent"},
		{"remaining-vm", true, true, true, true, true, "exited", "inconsistent"},
		{"orphan-running", true, false, true, true, true, "", "orphan-running"},
		{"orphan-empty", true, false, true, false, false, "", "orphan-empty"},
		{"daemon-unavailable", false, false, true, true, true, "", "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hosts := map[string]VM{}
			if tc.allocated {
				hosts[allocation] = VM{ID: allocation, Allocation: allocation, ContainerID: worker, Allocated: true, Populated: tc.populated, HostVerified: tc.verified, Tracked: true}
			}
			var docker []VM
			if tc.present {
				docker = []VM{{ID: worker, ContainerID: worker, Allocation: allocation, ContainerPresent: true, EngineState: tc.docker}}
			}
			result := reconcileVMs(hosts, docker, tc.available)
			if len(result) != 1 || result[0].State != tc.want {
				t.Fatalf("unexpected reconciliation: %+v", result)
			}
			if !tc.available && result[0].Orphan {
				t.Fatal("containerd failure was misclassified as an orphan")
			}
		})
	}
}
func TestInventoryDetectsDuplicateAllocationAndBindingMismatch(t *testing.T) {
	hosts := map[string]VM{"allocation": {ID: "allocation", Allocation: "allocation", ContainerID: "original", Allocated: true, Populated: true, HostVerified: true}}
	records := []VM{{ID: "replacement", ContainerID: "replacement", Allocation: "allocation", ContainerPresent: true, EngineState: "running"}, {ID: "original", ContainerID: "original", Allocation: "allocation", ContainerPresent: true, EngineState: "running"}}
	for _, v := range reconcileVMs(hosts, records, true) {
		if v.State != "inconsistent" {
			t.Fatalf("conflicting allocation accepted: %+v", v)
		}
	}
}
func TestResolveVMRefusesAmbiguityIncludingNameIDCollisions(t *testing.T) {
	snapshot := Inventory{VMs: []VM{{ID: "abcdef012345111", Name: "first", Allocation: "alloc1"}, {ID: "abcdef012345222", Name: "second", Allocation: "alloc2"}}}
	if _, err := ResolveVM(snapshot, "abcdef012345"); err == nil {
		t.Fatal("ambiguous prefix accepted")
	}
	if v, err := ResolveVM(snapshot, "first"); err != nil || v.Allocation != "alloc1" {
		t.Fatal(v, err)
	}
	snapshot.VMs[1].Name = snapshot.VMs[0].ID
	if _, err := ResolveVM(snapshot, snapshot.VMs[0].ID); err == nil {
		t.Fatal("name/ID conflict accepted")
	}
	if _, err := ResolveVM(snapshot, ""); err == nil {
		t.Fatal("empty selector accepted")
	}
}
func TestOrphanRecoveryRequiresOwnershipAbsenceAndExplicitLiveForce(t *testing.T) {
	now := time.Now()
	base := VM{Orphan: true, Tracked: true, CreatedAt: now.Add(-time.Hour)}
	for _, tc := range []struct {
		name        string
		edit        func(*VM)
		force, want bool
	}{
		{"old-empty", func(*VM) {}, false, true},
		{"untracked", func(v *VM) { v.Tracked = false }, true, false},
		{"container-present", func(v *VM) { v.ContainerPresent = true }, true, false},
		{"live-without-force", func(v *VM) { v.Populated = true }, false, false},
		{"live-explicit-force", func(v *VM) { v.Populated = true; v.ContainerID = "worker" }, true, true},
		{"fresh-reservation", func(v *VM) { v.CreatedAt = now }, true, false},
		{"known-deleted-worker", func(v *VM) { v.CreatedAt = now; v.ContainerID = "worker" }, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := base
			tc.edit(&v)
			err := canRecoverOrphan(v, tc.force, now)
			if (err == nil) != tc.want {
				t.Fatalf("recovery eligibility: %v", err)
			}
		})
	}
}
