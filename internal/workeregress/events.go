package workeregress

import (
	"context"
	"fmt"
	"github.com/docker/docker/api/types/container"
	"github.com/pacificbelt30/codex-dock/internal/engine"
	"github.com/pacificbelt30/codex-dock/internal/policyevents"
)

// Events reads bounded retained console history from the selected proxy. Shared
// proxies report observed source addresses; these are not authenticated run IDs.
func Events(ctx context.Context, socket, worker string, follow bool, visit func(policyevents.Event) error) error {
	s, err := Inspect(ctx, socket, worker)
	if err != nil {
		return err
	}
	if s.Proxy == "" {
		return fmt.Errorf("worker has no general-egress proxy")
	}
	cli, err := engine.NewClientAt(socket)
	if err != nil {
		return err
	}
	defer func() { _ = cli.Close() }()
	stream, err := cli.ContainerLogs(ctx, s.Proxy, container.LogsOptions{ShowStdout: true, ShowStderr: true, Tail: "1000", Follow: follow})
	if err != nil {
		return err
	}
	defer func() { _ = stream.Close() }()
	return policyevents.Read(stream, func(e policyevents.Event) error {
		if e.PolicyID == "" {
			e.PolicyID = s.PolicyID
		}
		return visit(e)
	})
}
