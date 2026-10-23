package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

func deliverProxyKey(ctx context.Context, c *client.Client, name string, spec proxyRunArgs) error {
	if spec.profileFingerprint == "" {
		return nil
	}
	if len(spec.profileKey) != 32 {
		return fmt.Errorf("encrypted profile is locked")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	attached, err := c.ContainerAttach(ctx, name, container.AttachOptions{Stream: true, Stdin: true})
	if err == nil {
		_ = attached.Conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		_, err = attached.Conn.Write(spec.profileKey)
		_ = attached.CloseWrite()
		attached.Close()
	}
	if err != nil {
		// Never leave a key-starved or partially initialized proxy running.
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer stopCancel()
		if stopErr := c.ContainerStop(stopCtx, name, container.StopOptions{}); stopErr != nil {
			return fmt.Errorf("proxy key transfer failed and proxy could not be stopped")
		}
		return fmt.Errorf("proxy key transfer failed")
	}
	return nil
}
