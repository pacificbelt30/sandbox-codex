package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/pacificbelt30/codex-dock/internal/workerpolicy"
	"time"
)

const lifecycleLabel = labelPrefix + "lifecycle"

func addWorkerPolicyLabels(labels map[string]string, opts RunOptions) error {
	if opts.Lifecycle != (workerpolicy.Lifecycle{}) {
		data, err := json.Marshal(opts.Lifecycle)
		if err != nil {
			return err
		}
		labels[lifecycleLabel] = string(data)
	}
	if len(opts.Filesystem.DenyRead)+len(opts.Filesystem.DenyWrite) > 0 {
		data, err := json.Marshal(opts.Filesystem)
		if err != nil {
			return err
		}
		if len(data) > 4096 {
			return fmt.Errorf("filesystem policy exceeds label limit")
		}
		labels[labelPrefix+"filesystem"] = string(data)
	}
	return nil
}

type ReapAction struct {
	Worker string `json:"worker"`
	Action string `json:"action"`
	Reason string `json:"reason"`
}

// Reap applies explicit lifecycle settings only. Idle means time since the last
// control-plane exec/session activity, not guest CPU or filesystem activity.
// dryRun returns the same decisions without changing workers.
func (m *Manager) Reap(ctx context.Context, now time.Time, dryRun bool) ([]ReapAction, error) {
	workers, err := m.ListContext(ctx, true)
	if err != nil {
		return nil, err
	}
	var actions []ReapAction
	var failures []error
	for _, w := range workers {
		if err := ctx.Err(); err != nil {
			return actions, err
		}
		v, err := m.cli.ContainerInspect(ctx, w.ID)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if v.Config == nil || v.State == nil || v.Config.Labels[lifecycleLabel] == "" {
			continue
		}
		var p workerpolicy.Lifecycle
		if err = json.Unmarshal([]byte(v.Config.Labels[lifecycleLabel]), &p); err != nil || p.Validate() != nil {
			failures = append(failures, fmt.Errorf("worker %s has invalid lifecycle policy", w.ID))
			continue
		}
		created, err := time.Parse(time.RFC3339Nano, v.Created)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		a := ReapAction{Worker: w.ID}
		if v.State.Running {
			if p.MaxLifetime > 0 && now.Sub(created) >= p.MaxLifetime {
				a.Action, a.Reason = "stop", "max_lifetime"
			} else if p.IdleTimeout > 0 {
				var active time.Time
				if value := v.Config.Labels[labelPrefix+"last-activity"]; value != "" {
					if t, err := time.Parse(time.RFC3339Nano, value); err == nil {
						active = t
					} else {
						failures = append(failures, err)
						continue
					}
				} else {
					failures = append(failures, fmt.Errorf("worker %s: node does not report idle activity; upgrade the node", w.ID))
					continue
				}
				if now.Sub(active) >= p.IdleTimeout {
					a.Action, a.Reason = "stop", "idle_timeout"
				}
			}
		} else if v.State.Status == "exited" && p.Retention > 0 {
			finished, err := time.Parse(time.RFC3339Nano, v.State.FinishedAt)
			if err != nil {
				failures = append(failures, err)
				continue
			}
			if now.Sub(finished) >= p.Retention {
				a.Action, a.Reason = "remove", "retention"
			}
		}
		if a.Action == "" {
			continue
		}
		actions = append(actions, a)
		if dryRun {
			continue
		}
		if a.Action == "stop" {
			err = m.StopContext(ctx, w.ID, 10)
		} else {
			err = m.RemoveContext(ctx, w.ID, false)
		}
		if err != nil {
			failures = append(failures, fmt.Errorf("%s %s: %w", a.Action, w.ID, err))
		}
	}
	return actions, errors.Join(failures...)
}
