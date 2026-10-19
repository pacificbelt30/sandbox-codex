//go:build linux

package engine

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/containerd/containerd/v2/pkg/cio"
)

// The shim's binary logger owns output and the stdin keepalive independently
// of this service. There are no service-owned FIFO readers to break on restart.
type loggingIO struct{ config cio.Config }

func (l *loggingIO) Config() cio.Config { return l.config }
func (*loggingIO) Cancel()              {}
func (*loggingIO) Wait()                {}
func (*loggingIO) Close() error         { return nil }
func (b *Backend) logIO(r *record) (cio.Creator, error) {
	return b.logIOAt(b.path(r.ID), r.ID, r.Config.Tty, r.Config.OpenStdin, 0)
}
func (b *Backend) logIOAt(dir, id string, tty, stdin bool, limit int64) (cio.Creator, error) {
	input := ""
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	if stdin {
		input = filepath.Join(dir, "stdin.fifo")
		if info, err := os.Lstat(input); err == nil {
			if info.Mode()&os.ModeNamedPipe == 0 {
				return nil, fmt.Errorf("stdin path is not a FIFO")
			}
		} else if os.IsNotExist(err) {
			if err = syscall.Mkfifo(input, 0600); err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	u := &url.URL{Scheme: "binary", Path: b.opts.Logger}
	q := url.Values{}
	q.Set("--state", dir)
	q.Set("--id", id)
	if limit > 0 {
		q.Set("--max-bytes", strconv.FormatInt(limit, 10))
	}
	u.RawQuery = q.Encode()
	return func(string) (cio.IO, error) {
		return &loggingIO{config: cio.Config{Stdin: input, Stdout: u.String(), Stderr: u.String(), Terminal: tty}}, nil
	}, nil
}
