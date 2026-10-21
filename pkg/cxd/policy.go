package cxd

import (
	"context"
	"fmt"
	"github.com/pacificbelt30/codex-dock/internal/networkpolicy"
	"github.com/pacificbelt30/codex-dock/internal/policyevents"
	"github.com/pacificbelt30/codex-dock/internal/sandbox"
	"github.com/pacificbelt30/codex-dock/internal/workeregress"
	"time"
)

type PolicySnapshot = workeregress.Snapshot
type PolicyDecision = networkpolicy.Decision
type PolicyEvent = policyevents.Event
type PolicyPreset = workeregress.Preset

func PolicyPresets() []PolicyPreset { return workeregress.Presets() }

// Quarantine stops ALL execution and network access for one managed worker and
// durably prevents restart. Workspace and logs remain; recreate the worker with
// a new RequestID to apply a new policy. Retrying is safe. An error means that
// completion is unconfirmed, even if the worker has already stopped.
func (c *Client) Quarantine(ctx context.Context, worker string) error {
	return classify(c.manager.QuarantineContext(ctx, worker))
}
func (c *Client) Policy(ctx context.Context, worker string) (PolicySnapshot, error) {
	return workeregress.Inspect(ctx, c.opts.NodeSocket, worker)
}
func (c *Client) ExplainPolicy(ctx context.Context, worker, destination string, resolve bool) (PolicyDecision, error) {
	s, err := c.Policy(ctx, worker)
	if err != nil {
		return PolicyDecision{}, err
	}
	if s.QuarantinePending {
		return PolicyDecision{}, fmt.Errorf("quarantine stop not confirmed; retry Quarantine for this worker")
	}
	if s.Quarantined {
		return PolicyDecision{Code: "worker_quarantined", Complete: true}, nil
	}
	if !s.GeneralEgress {
		return PolicyDecision{Code: "general_egress_disabled", Complete: true}, nil
	}
	return networkpolicy.Explain(ctx, s.Rules, destination, resolve, nil)
}
func (c *Client) PolicyEvents(ctx context.Context, worker string, follow bool, visit func(PolicyEvent) error) error {
	return workeregress.Events(ctx, c.opts.NodeSocket, worker, follow, visit)
}

type ReapAction = sandbox.ReapAction

func (c *Client) Reap(ctx context.Context, dryRun bool) ([]ReapAction, error) {
	return c.manager.Reap(ctx, time.Now(), dryRun)
}
