package cxd

import (
	"context"
	"github.com/pacificbelt30/codex-dock/internal/sandbox"
	"io"
)

// Session streams a PTY until Close, cancellation or process exit. Unlike a
// background Job, a streaming session intentionally ends on disconnection.
type Session struct {
	ID     string
	Worker string
	Input  io.WriteCloser
	cancel context.CancelFunc
	done   chan struct{}
	code   int64
	err    error
	client *Client
}

func (c *Client) StartSession(ctx context.Context, worker string, req ExecRequest) (*Session, error) {
	ctx, cancel := context.WithCancel(ctx)
	reader, writer := io.Pipe()
	s := &Session{Worker: worker, Input: writer, cancel: cancel, done: make(chan struct{}), client: c}
	started := make(chan string, 1)
	go func() {
		defer close(s.done)
		defer cancel()
		defer func() { _ = reader.Close() }()
		s.code, s.err = c.manager.ExecWithOptions(ctx, worker, sandbox.ExecOptions{Argv: req.Argv, Stdin: reader, Stdout: req.Stdout, Stderr: req.Stderr, Env: req.Env, WorkingDir: req.WorkingDir, TTY: true, OnStart: func(id string) { started <- id }})
	}()
	select {
	case id := <-started:
		s.ID = id
		return s, nil
	case <-s.done:
		select {
		case id := <-started:
			s.ID = id
			return s, nil
		default:
		}
		cancel()
		_ = writer.Close()
		return nil, s.err
	case <-ctx.Done():
		select {
		case id := <-started:
			s.ID = id
			return s, nil
		default:
		}
		cancel()
		_ = writer.Close()
		return nil, ctx.Err()
	}
}
func (s *Session) Resize(ctx context.Context, width, height uint) error {
	return s.client.manager.ResizeSession(ctx, s.Worker, s.ID, width, height)
}
func (s *Session) Wait() (int64, error) { <-s.done; return s.code, s.err }
func (s *Session) Close() error         { s.cancel(); return s.Input.Close() }
