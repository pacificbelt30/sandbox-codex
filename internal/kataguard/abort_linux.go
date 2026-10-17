//go:build linux

package kataguard

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// AbortCreate is not an RPC action. The native engine invokes it only after a
// failed Create/Start, while user code is still behind the bootstrap gate.
// It never targets a PID or a cgroup outside the attested reservation.
func (g *Guard) AbortCreate(ctx context.Context, allocation, worker string) error {
	if err := g.Validate(ctx, g.manifest.Runtime); err != nil {
		return err
	}
	r, err := g.readRecord(allocation)
	if err != nil {
		return err
	}
	if r.Worker != "" && r.Worker != worker {
		return fmt.Errorf("failed-start allocation ownership mismatch")
	}
	path, err := g.group(allocation)
	if err != nil {
		return err
	}
	if _, err = os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	processes, err := g.scanProcesses(ctx, allocation, worker, false)
	if err != nil {
		return err
	}
	if len(processes) == 0 {
		return nil
	}
	return os.WriteFile(filepath.Join(path, "cgroup.kill"), []byte("1"), 0600)
}
