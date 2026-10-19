package cxd

import (
	"context"
	"github.com/pacificbelt30/codex-dock/internal/engine"
	"github.com/pacificbelt30/codex-dock/internal/environmentlock"
)

type EnvironmentLock = environmentlock.Lock

func (c *Client) LockEnvironment(ctx context.Context, root, image string, sources []string) (EnvironmentLock, error) {
	cli, err := engine.NewClientAt(c.opts.NodeSocket)
	if err != nil {
		return EnvironmentLock{}, err
	}
	defer func() { _ = cli.Close() }()
	info, err := cli.ImageInspect(ctx, image)
	if err != nil {
		return EnvironmentLock{}, err
	}
	return environmentlock.New(root, image, info.ID, sources)
}
func LoadEnvironmentLock(file, root, image string) (EnvironmentLock, error) {
	return environmentlock.Load(file, root, image)
}
func (c *Client) LaunchLocked(ctx context.Context, req LaunchRequest, file string) (Worker, error) {
	lock, err := environmentlock.Load(file, req.ProjectDir, req.Image)
	if err != nil {
		return Worker{}, err
	}
	req.Environment.ImageID = lock.ImageID
	return c.Launch(ctx, req)
}
