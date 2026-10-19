package kataguard

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// VM describes a containerd record and its independently observed host envelope.
// HostVerified proves process identity/confinement, not guest responsiveness.
type VM struct {
	ID               string            `json:"id"`
	Name             string            `json:"name,omitempty"`
	ResourceProfile  string            `json:"resource_profile,omitempty"`
	Allocation       string            `json:"allocation,omitempty"`
	ContainerID      string            `json:"container_id,omitempty"`
	ContainerPresent bool              `json:"container_present"`
	EngineState      string            `json:"engine_state,omitempty"`
	State            string            `json:"state"`
	Cgroup           string            `json:"cgroup,omitempty"`
	Allocated        bool              `json:"allocated"`
	Populated        bool              `json:"populated"`
	Tracked          bool              `json:"tracked"`
	HostVerified     bool              `json:"host_verified"`
	Orphan           bool              `json:"orphan"`
	CreatedAt        time.Time         `json:"created_at,omitempty"`
	Processes        []VMProcess       `json:"processes,omitempty"`
	Resources        map[string]string `json:"resources,omitempty"`
	Problems         []string          `json:"problems,omitempty"`
}
type VMProcess struct {
	PID  int    `json:"pid"`
	Kind string `json:"kind"`
}
type Inventory struct {
	ObservedAt      time.Time `json:"observed_at"`
	EngineAvailable bool      `json:"engine_available"`
	Problems        []string  `json:"problems,omitempty"`
	VMs             []VM      `json:"vms"`
}

func classifyVM(v *VM, available bool) {
	switch {
	case !available:
		v.State = "unknown"
	case !v.ContainerPresent:
		v.Orphan = true
		if v.Populated {
			v.State = "orphan-running"
		} else {
			v.State = "orphan-empty"
		}
	case len(v.Problems) > 0:
		v.State = "inconsistent"
	case v.EngineState == "running" || v.EngineState == "paused":
		if v.Allocated && v.Populated && v.HostVerified {
			v.State = v.EngineState
		} else {
			v.State = "inconsistent"
			v.Problems = append(v.Problems, "containerd reports running but the verified VM is missing")
		}
	case v.Populated:
		v.State = "inconsistent"
		v.Problems = append(v.Problems, "VM processes remain while containerd reports "+v.EngineState)
	default:
		v.State = v.EngineState
		if v.State == "" {
			v.State = "unknown"
		}
	}
}

type VMNotFoundError struct{ Reference string }

func (e *VMNotFoundError) Error() string { return fmt.Sprintf("VM %q not found", e.Reference) }

// ResolveVM never prefers a container name over a conflicting allocation/ID.
func ResolveVM(inventory Inventory, value string) (VM, error) {
	if value == "" {
		return VM{}, fmt.Errorf("VM name or ID is required")
	}
	var matches []VM
	for _, v := range inventory.VMs {
		match := v.ID == value || v.Name == value || v.Allocation == value || v.ContainerID == value
		if len(value) >= 12 {
			match = match || strings.HasPrefix(v.ID, value) || v.Allocation != "" && strings.HasPrefix(v.Allocation, value) || v.ContainerID != "" && strings.HasPrefix(v.ContainerID, value)
		}
		if match {
			matches = append(matches, v)
		}
	}
	if len(matches) == 0 {
		return VM{}, &VMNotFoundError{Reference: value}
	}
	if len(matches) != 1 {
		return VM{}, fmt.Errorf("ambiguous VM name or ID %q", value)
	}
	return matches[0], nil
}
func sortVMs(vms []VM) { sort.Slice(vms, func(i, j int) bool { return vms[i].ID < vms[j].ID }) }

func reconcileVMs(hosts map[string]VM, engine []VM, available bool) []VM {
	result := make([]VM, 0, len(hosts)+len(engine))
	used := map[string]bool{}
	counts := map[string]int{}
	for _, v := range engine {
		if v.Allocation != "" {
			counts[v.Allocation]++
		}
	}
	for _, record := range engine {
		vm := record
		if host, ok := hosts[record.Allocation]; ok {
			vm = host
			vm.ID = record.ID
			vm.Name = record.Name
			vm.ResourceProfile = record.ResourceProfile
			vm.ContainerPresent = true
			vm.EngineState = record.EngineState
			vm.Problems = append(append([]string(nil), host.Problems...), record.Problems...)
			if host.ContainerID != "" && host.ContainerID != record.ContainerID {
				vm.Problems = append(vm.Problems, "reservation belongs to another containerd worker")
			}
			vm.ContainerID = record.ContainerID
		}
		if counts[record.Allocation] > 1 {
			vm.Problems = append(vm.Problems, "duplicate containerd allocation")
		}
		used[record.Allocation] = true
		classifyVM(&vm, available)
		result = append(result, vm)
	}
	for id, host := range hosts {
		if !used[id] {
			classifyVM(&host, available)
			result = append(result, host)
		}
	}
	sortVMs(result)
	return result
}
func canRecoverOrphan(vm VM, force bool, now time.Time) error {
	if !vm.Orphan || vm.ContainerPresent {
		return fmt.Errorf("VM still has a containerd record; use worker stop/rm")
	}
	if !vm.Tracked {
		return fmt.Errorf("refusing recovery without a trusted allocation record")
	}
	if vm.Populated && !force {
		return fmt.Errorf("orphan VM is running; use --force to stop its verified processes")
	}
	// A reserve/create window has no immutable containerd ID yet. Keep a grace period
	// before reclaiming that empty reservation; normal stop/rm does not wait.
	if vm.ContainerID == "" && now.Sub(vm.CreatedAt) < 5*time.Minute {
		return fmt.Errorf("reservation is recent; wait five minutes before orphan recovery")
	}
	return nil
}
