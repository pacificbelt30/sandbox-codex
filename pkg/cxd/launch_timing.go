package cxd

import (
	"context"
	"time"

	"github.com/pacificbelt30/codex-dock/internal/launchtiming"
)

// LaunchPhase is elapsed wall time in one sequential host-side launch phase.
// Names describe this implementation's phases and may grow or change between
// versions; they are diagnostics, not lifecycle events or readiness signals.
type LaunchPhase struct {
	Name     string        `json:"name"`
	Duration time.Duration `json:"duration_ns"`
}

// LaunchTimings includes all of Launch, including failure cleanup and request
// confirmation. It excludes Open, image builds and any later agent inference.
// On an idempotent retry, only the phases actually executed are present.
type LaunchTimings struct {
	Duration time.Duration `json:"duration_ns"`
	Phases   []LaunchPhase `json:"phases"`
}

// LaunchWithTimings has the same request identity, lifecycle and error contract
// as Launch. Measurements stay in this process and contain no request values,
// worker IDs, filesystem paths, commands, tokens or error text. A failed launch
// returns its partial timings along with the ordinary worker and error.
func (c *Client) LaunchWithTimings(ctx context.Context, req LaunchRequest) (Worker, LaunchTimings, error) {
	ctx, recorder := launchtiming.Begin(ctx)
	w, err := c.Launch(ctx, req)
	total, phases := recorder.Finish()
	timings := LaunchTimings{Duration: total, Phases: make([]LaunchPhase, 0, len(phases))}
	for _, phase := range phases {
		timings.Phases = append(timings.Phases, LaunchPhase{Name: phase.Name, Duration: phase.Duration})
	}
	return w, timings, err
}
