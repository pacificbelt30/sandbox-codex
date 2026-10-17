//go:build linux

package engine

import (
	"context"
	"errors"
	"fmt"
	cd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/errdefs"
	"time"
)

func (b *Backend) failedStart(ctx context.Context, r *record, c cd.Container, cause error) error {
	b.closePorts(r.ID)
	if h := b.hubs[r.ID]; h != nil {
		_ = h.Close()
		delete(b.hubs, r.ID)
	}
	r.FailedCreate = true
	r.ExitCode = 125
	r.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := b.save(r); err != nil {
		return errors.Join(cause, err)
	}
	return errors.Join(cause, b.abortTask(ctx, r, c))
}
func (b *Backend) abortTask(ctx context.Context, r *record, c cd.Container) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if r.Host.Runtime != "runc" {
		if b.opts.AbortCreate == nil {
			return fmt.Errorf("failed-start verifier missing")
		}
		if err := b.opts.AbortCreate(ctx, r.Config.Labels["codex-dock.allocation"], r.ID); err != nil {
			return err
		}
	} else if task, err := c.Task(ctx, nil); err == nil {
		if _, err = task.Delete(ctx, cd.WithProcessKill); err != nil && !errdefs.IsNotFound(err) {
			return err
		}
	} else if !errdefs.IsNotFound(err) {
		return err
	}
	for {
		_, err := c.Task(ctx, nil)
		if errdefs.IsNotFound(err) {
			return nil
		}
		if ctx.Err() != nil {
			return fmt.Errorf("failed task cleanup retained for retry: %w", ctx.Err())
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}
