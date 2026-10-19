package sandbox

import (
	"context"
	"fmt"
	"io"
	"time"
)

func (m *Manager) waitEnvironment(ctx context.Context, id, readyPath string, timeout time.Duration) error {
	if timeout == 0 {
		timeout = time.Minute
	}
	probe, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		code, err := m.Exec(probe, id, []string{"test", "-f", readyPath}, nil, io.Discard, io.Discard)
		if err == nil && code == 0 {
			return nil
		}
		select {
		case <-probe.Done():
			return fmt.Errorf("waiting for environment readiness: %w", probe.Err())
		case <-ticker.C:
		}
		v, err := m.InspectContext(probe, id)
		if err != nil {
			return err
		}
		if v.Status != "running" {
			return fmt.Errorf("worker exited before readiness; inspect its logs")
		}
	}
}
