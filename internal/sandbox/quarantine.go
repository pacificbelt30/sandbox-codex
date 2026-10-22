package sandbox

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/pacificbelt30/codex-dock/internal/workerpolicy"
)

// QuarantineContext immediately stops the selected managed worker and durably
// forbids restart at the node. Its container, workspace and logs are retained.
// This ends ALL work and network access, including the fixed AI routes. To
// resume under a new immutable policy, create a new worker with a new request ID.
// Retrying is safe; an error never establishes that quarantine completed.
func (m *Manager) QuarantineContext(parent context.Context, worker string) error {
	ctx, cancel := context.WithTimeout(parent, 120*time.Second)
	defer cancel()
	v, err := m.cli.ContainerInspect(ctx, worker)
	if err != nil {
		return err
	}
	if v.ContainerJSONBase == nil || v.Config == nil || v.HostConfig == nil || v.Config.Labels[labelManaged] != "true" || v.Config.Labels[labelIsolation] != RuntimeKata && v.Config.Labels[labelIsolation] != RuntimeRunc {
		return fmt.Errorf("quarantine requires a managed worker")
	}
	info, err := m.cli.Info(ctx)
	if err != nil {
		return err
	}
	supported := false
	for _, feature := range info.DriverStatus {
		if feature == [2]string{"cxd.quarantine", "1"} {
			supported = true
		}
	}
	if !supported {
		return fmt.Errorf("durable quarantine requires an updated Oaiso node; stop the worker explicitly until it is upgraded")
	}
	unlock, err := m.lockCleanup(ctx, v.ID)
	if err != nil {
		return err
	}
	defer unlock()
	stopErr := m.jobRequest(ctx, http.MethodPost, "/containers/"+url.PathEscape(v.ID)+"/quarantine", nil)
	// Even an uncertain stop must revoke the worker's AI placeholder through
	// the checked revocation path, even if the caller stopped waiting.
	revokeErr := m.revokeWorkerTokenChecked(v)
	if stopErr != nil {
		return errors.Join(fmt.Errorf("quarantine not confirmed; retry the same worker: %w", stopErr), revokeErr)
	}
	confirmed, err := m.cli.ContainerInspect(ctx, v.ID)
	if err != nil {
		return errors.Join(fmt.Errorf("quarantine status not confirmed; retry the same worker: %w", err), revokeErr)
	}
	if confirmed.ContainerJSONBase == nil || confirmed.ID != v.ID || confirmed.State == nil || confirmed.State.Running || confirmed.State.Paused || confirmed.State.Status != "created" && confirmed.State.Status != "exited" || confirmed.Config == nil || confirmed.Config.Labels[workerpolicy.QuarantinedLabel] != "true" {
		return errors.Join(fmt.Errorf("quarantine stop/status not confirmed; retry the same worker"), revokeErr)
	}
	if v.Config.Labels[labelIsolation] == RuntimeKata && v.Config.Labels[labelAllocation] != "" {
		err = m.releaseKataAllocation(ctx, v.Config.Labels[labelAllocation], v.ID)
	}
	return errors.Join(err, revokeErr)
}
