//go:build linux

package kataguard

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// Retire is called by the native backend only after containerd confirms the
// guest task has exited, before Task.Delete sends shim Shutdown. Kata 4.2's
// embedded VMM tries to destroy its own occupied sandbox cgroup. A sibling leaf
// empties that sandbox while keeping every thread inside the SAME CPU/memory/
// swap/PID envelope. This never moves a process outside its allocation.
//
// The engine serializes task mutations. Do not take admissionLock here: an
// inventory request can hold it while waiting for the engine's task lock.
func (g *Guard) Retire(ctx context.Context, allocation, worker string) error {
	if err := g.Validate(ctx, g.manifest.Runtime); err != nil {
		return err
	}
	r, err := g.readRecord(allocation)
	if err != nil {
		return err
	}
	if r.Worker != "" && r.Worker != worker {
		return fmt.Errorf("retirement allocation ownership mismatch")
	}
	processes, err := g.scanProcesses(ctx, allocation, worker, false)
	if err != nil {
		return err
	}
	if len(processes) == 0 {
		return nil
	}
	parent, err := g.group(allocation)
	if err != nil {
		return err
	}
	target := filepath.Join(parent, "supervisor")
	if err = os.Mkdir(target, 0755); err != nil && !os.IsExist(err) {
		return err
	}
	if err = trustedPath(target, true); err != nil {
		return err
	}
	for _, process := range processes {
		if process.Kind != "runtime" && process.Kind != "logger" {
			return fmt.Errorf("unexpected Dragonball occupant")
		}
		if err = g.moveRetiredProcess(ctx, allocation, worker, process.PID, target); err != nil {
			return err
		}
	}

	return g.checkLimits(parent)
}

func (g *Guard) moveRetiredProcess(ctx context.Context, allocation, worker string, pid int, target string) error {
	fd, err := unix.PidfdOpen(pid, 0)
	if err == unix.ESRCH {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { _ = unix.Close(fd) }()
	if err = unix.PidfdSendSignal(fd, unix.SIGSTOP, nil, 0); err == unix.ESRCH {
		return nil
	} else if err != nil {
		return err
	}
	defer func() { _ = unix.PidfdSendSignal(fd, unix.SIGCONT, nil, 0) }()
	if _, err = g.scanProcesses(ctx, allocation, worker, false); err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(target, "cgroup.procs"), []byte(strconv.Itoa(pid)), 0600); err != nil {
		return err
	}
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cgroup", pid))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(data)) != "0::"+strings.TrimPrefix(target, "/sys/fs/cgroup") {
		return fmt.Errorf("retired process escaped its envelope")
	}
	return nil
}
