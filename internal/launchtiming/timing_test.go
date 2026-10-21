package launchtiming

import (
	"context"
	"testing"
	"time"
)

func TestSequentialDurationsPartitionTotal(t *testing.T) {
	ctx, r := Begin(context.Background())
	r.next("vm_start", r.started.Add(2*time.Second))
	r.next("worker_cleanup", r.started.Add(5*time.Second))
	// Use the same deterministic clock for the last boundary.
	r.next("", r.started.Add(9*time.Second))
	if len(r.phases) != 3 || r.phases[0] != (Phase{"request_validation", 2 * time.Second}) || r.phases[1] != (Phase{"vm_start", 3 * time.Second}) || r.phases[2] != (Phase{"worker_cleanup", 4 * time.Second}) {
		t.Fatalf("unexpected phase partition: %+v", r.phases)
	}
	if ctx.Value(key{}) != r {
		t.Fatal("recording context lost its recorder")
	}
}

func TestRecorderIsolationAndNoop(t *testing.T) {
	Next(context.Background(), "not-recorded")
	first, a := Begin(context.Background())
	second, b := Begin(context.Background())
	Next(first, "vm_start")
	Next(second, "request_abort")
	total, phases := a.Finish()
	var sum time.Duration
	for _, phase := range phases {
		if phase.Duration < 0 {
			t.Fatalf("negative elapsed time: %+v", phase)
		}
		sum += phase.Duration
	}
	if sum != total || total < 0 || len(phases) != 2 || phases[1].Name != "vm_start" || b.name != "request_abort" {
		t.Fatalf("recorders mixed or lost time: total=%v phases=%+v other=%s", total, phases, b.name)
	}
}
