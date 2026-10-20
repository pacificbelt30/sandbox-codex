//go:build linux

package engine

import (
	"testing"
	"time"
)

func TestIdleCollectionProtectsPendingStreamingAndRecoveredJobs(t *testing.T) {
	b := &Backend{execs: map[string]*execution{
		"pending": {Container: "worker"},
		"stream":  {Container: "worker", Started: true, Running: true},
		"job":     {Container: "worker", Started: true, Running: true, Detached: true},
		"done":    {Container: "worker", Started: true},
		"other":   {Container: "other", Running: true},
	}}
	before := time.Now().Add(-time.Hour)
	last := before.Add(-time.Hour)
	if n := b.activeExecutions("worker"); n != 3 || idleEligible(last, before, n) {
		t.Fatalf("active sessions not protected: %d", n)
	}
	delete(b.execs, "pending")
	b.execs["stream"].Running = false
	b.execs["job"].Running = false
	if n := b.activeExecutions("worker"); n != 0 || !idleEligible(last, before, n) {
		t.Fatal("completed sessions block collection")
	}
	if idleEligible(before.Add(time.Second), before, 0) || idleEligible(time.Time{}, before, 0) {
		t.Fatal("new or unknown activity was collected")
	}
}
