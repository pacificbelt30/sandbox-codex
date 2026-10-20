//go:build linux

package engine

import "time"

// Called with b.mu held. Pending execs protect the worker until their bounded
// creation lease expires; detached jobs are restored before the node serves.
func (b *Backend) activeExecutions(worker string) int {
	n := 0
	for _, e := range b.execs {
		if e.Container == worker && (!e.Started || e.Running) {
			n++
		}
	}
	return n
}

func idleEligible(last, before time.Time, active int) bool {
	return !last.IsZero() && !last.After(before) && active == 0
}
