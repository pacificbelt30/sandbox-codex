package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/pacificbelt30/codex-dock/internal/kataguard"
)

type vmNode interface {
	Inventory(context.Context) (kataguard.Inventory, error)
	RecoverOrphan(context.Context, string, bool) error
}

func (m *Manager) vmNode() (vmNode, error) {
	node, err := m.kataNode()
	if err != nil {
		return nil, err
	}
	api, ok := node.(vmNode)
	if !ok {
		return nil, fmt.Errorf("kata node requires an update for VM management")
	}
	return api, nil
}
func (m *Manager) VMInventory(ctx context.Context) (kataguard.Inventory, error) {
	node, err := m.vmNode()
	if err != nil {
		return kataguard.Inventory{}, err
	}
	return node.Inventory(ctx)
}
func (m *Manager) VMInspect(ctx context.Context, value string) (kataguard.VM, error) {
	inventory, err := m.VMInventory(ctx)
	if err != nil {
		return kataguard.VM{}, err
	}
	return kataguard.ResolveVM(inventory, value)
}
func (m *Manager) VMStart(ctx context.Context, value string) error {
	inventory, err := m.VMInventory(ctx)
	if err != nil {
		return err
	}
	if !inventory.EngineAvailable {
		return fmt.Errorf("containerd is unavailable; cannot start a VM")
	}
	vm, err := kataguard.ResolveVM(inventory, value)
	if err != nil {
		return err
	}
	if !vm.ContainerPresent {
		return fmt.Errorf("orphan VM has no retained worker to start; remove it and create a new worker")
	}
	return m.Start(vm.ContainerID)
}
func (m *Manager) VMStop(ctx context.Context, value string, timeout int, force bool) error {
	inventory, err := m.VMInventory(ctx)
	if err != nil {
		return err
	}
	if !inventory.EngineAvailable {
		return fmt.Errorf("containerd is unavailable; cannot safely stop a VM")
	}
	vm, err := kataguard.ResolveVM(inventory, value)
	if err != nil {
		return err
	}
	if vm.ContainerPresent {
		return m.Stop(vm.ContainerID, timeout)
	}
	node, err := m.vmNode()
	if err != nil {
		return err
	}
	return node.RecoverOrphan(ctx, vm.Allocation, force)
}
func (m *Manager) VMRemove(ctx context.Context, value string, force bool) error {
	inventory, err := m.VMInventory(ctx)
	if err != nil {
		return err
	}
	if !inventory.EngineAvailable {
		return fmt.Errorf("containerd is unavailable; cannot safely remove a VM")
	}
	vm, err := kataguard.ResolveVM(inventory, value)
	if err != nil {
		var missing *kataguard.VMNotFoundError
		if !errors.As(err, &missing) {
			return err
		}
		if record, readErr := m.findCleanup(value); readErr == nil && record.Labels[labelIsolation] == RuntimeKata {
			return m.Remove(record.ID, force)
		}
		return err
	}
	if vm.ContainerPresent {
		return m.Remove(vm.ContainerID, force)
	}
	if vm.ContainerID != "" {
		if _, readErr := m.readCleanup(vm.ContainerID); readErr == nil {
			return m.Remove(vm.ContainerID, force)
		} else if !os.IsNotExist(readErr) {
			return readErr
		}
	}
	node, err := m.vmNode()
	if err != nil {
		return err
	}
	return node.RecoverOrphan(ctx, vm.Allocation, force)
}

// VMPrune only recovers empty orphan reservations. It never kills live VMs or
// deletes stopped containerd workers; those remain explicitly addressable by rm.
func (m *Manager) VMPrune(ctx context.Context) ([]string, error) {
	inventory, err := m.VMInventory(ctx)
	if err != nil {
		return nil, err
	}
	if !inventory.EngineAvailable {
		return nil, fmt.Errorf("containerd is unavailable; cannot identify orphan allocations")
	}
	node, err := m.vmNode()
	if err != nil {
		return nil, err
	}
	var removed []string
	var failures error
	for _, vm := range inventory.VMs {
		if !vm.Orphan || vm.Populated {
			continue
		}
		if err := node.RecoverOrphan(ctx, vm.Allocation, false); err != nil {
			failures = errors.Join(failures, fmt.Errorf("allocation %s: %w", vm.Allocation, err))
			continue
		}
		removed = append(removed, vm.Allocation)
	}
	return removed, failures
}
