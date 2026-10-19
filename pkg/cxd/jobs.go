package cxd

import (
	"context"
	"github.com/pacificbelt30/codex-dock/internal/sandbox"
	"time"
)

type Job = sandbox.Job
type JobOutput = sandbox.JobOutput

func (c *Client) StartJob(ctx context.Context, worker string, req ExecRequest) (Job, error) {
	return c.manager.StartJob(ctx, worker, sandbox.ExecOptions{Argv: req.Argv, Env: req.Env, WorkingDir: req.WorkingDir, TTY: req.TTY, Stdin: req.Stdin})
}
func (c *Client) Job(ctx context.Context, worker, id string) (Job, error) {
	return c.manager.Job(ctx, worker, id)
}
func (c *Client) ListJobs(ctx context.Context, worker string) ([]Job, error) {
	return c.manager.ListJobs(ctx, worker)
}
func (c *Client) JobLogs(ctx context.Context, worker, id string, offset int64) (JobOutput, error) {
	return c.manager.JobLogs(ctx, worker, id, offset)
}
func (c *Client) KillJob(ctx context.Context, worker, id string) error {
	return c.manager.KillJob(ctx, worker, id)
}
func (c *Client) WaitJob(ctx context.Context, worker, id string) (Job, error) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		j, err := c.Job(ctx, worker, id)
		if err != nil || !j.Running {
			return j, err
		}
		select {
		case <-ctx.Done():
			return j, ctx.Err()
		case <-ticker.C:
		}
	}
}
