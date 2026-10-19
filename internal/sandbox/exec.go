package sandbox

import (
	"context"
	"fmt"
	"io"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
)

// Exec runs argv in an existing managed worker. It returns the guest exit code
// separately from transport errors so callers can preserve script semantics.
// Closing ctx closes the stream; the node cancels and kills the exec process.
func (m *Manager) Exec(ctx context.Context, name string, argv []string, stdin io.Reader, stdout, stderr io.Writer) (int64, error) {
	if len(argv) == 0 {
		return 0, fmt.Errorf("exec requires a command")
	}
	id, err := m.resolveID(name)
	if err != nil {
		return 0, err
	}
	v, err := m.cli.ContainerInspect(ctx, id)
	if err != nil {
		return 0, err
	}
	if v.State == nil || !v.State.Running {
		return 0, fmt.Errorf("worker %s is not running", name)
	}
	if v.Config == nil || v.Config.Labels[labelManaged] != "true" {
		return 0, fmt.Errorf("worker %s is not managed", name)
	}
	if v.Config.Labels[labelIsolation] == RuntimeKata {
		if err := m.CheckRuntime(ctx, RunOptions{Runtime: RuntimeKata, KataRuntime: v.Config.Labels[labelRuntime], ResourceProfile: v.Config.Labels[labelPrefix+"resource-profile"]}); err != nil {
			return 0, err
		}
		if err := m.validateKataWorker(ctx, id); err != nil {
			return 0, err
		}
		if err := m.verifyKataGuest(ctx, id, false); err != nil {
			return 0, err
		}
	}
	created, err := m.cli.ContainerExecCreate(ctx, id, container.ExecOptions{Cmd: argv, AttachStdin: stdin != nil, AttachStdout: true, AttachStderr: true, WorkingDir: "/workspace"})
	if err != nil {
		return 0, err
	}
	stream, err := m.cli.ContainerExecAttach(ctx, created.ID, container.ExecAttachOptions{})
	if err != nil {
		return 0, err
	}
	defer stream.Close()
	stop := context.AfterFunc(ctx, stream.Close)
	defer stop()
	if stdin != nil {
		go func() { _, _ = io.Copy(stream.Conn, stdin); _ = stream.CloseWrite() }()
	}
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	_, copyErr := stdcopy.StdCopy(stdout, stderr, stream.Reader)
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if copyErr != nil {
		return 0, copyErr
	}
	status, err := m.cli.ContainerExecInspect(ctx, created.ID)
	if err != nil {
		return 0, err
	}
	if status.Running {
		return 0, fmt.Errorf("exec stream ended before process exit")
	}
	return int64(status.ExitCode), nil
}

// WaitContext waits for a managed worker to stop and returns its exit code.
func (m *Manager) WaitContext(ctx context.Context, name string) (int64, error) {
	id, err := m.resolveID(name)
	if err != nil {
		return 0, err
	}
	statusCh, errCh := m.cli.ContainerWait(ctx, id, container.WaitConditionNotRunning)
	for {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case err, ok := <-errCh:
			if ok && err != nil {
				return 0, err
			}
			errCh = nil
		case status, ok := <-statusCh:
			if !ok {
				return 0, fmt.Errorf("worker wait stream closed")
			}
			if status.Error != nil {
				return 0, fmt.Errorf("waiting for worker: %s", status.Error.Message)
			}
			v, err := m.cli.ContainerInspect(ctx, id)
			if err != nil {
				return status.StatusCode, err
			}
			return status.StatusCode, m.revokeWorkerTokenChecked(v)
		}
	}
}
