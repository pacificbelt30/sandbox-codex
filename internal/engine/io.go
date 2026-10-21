//go:build linux

package engine

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"

	"github.com/containerd/containerd/v2/pkg/cio"
)

// The shim's binary logger owns output and the stdin keepalive independently
// of this service. There are no service-owned FIFO readers to break on restart.
type loggingIO struct {
	config    cio.Config
	eof       *os.File
	closeOnce sync.Once
	closeErr  error
}

func (l *loggingIO) CloseInput() error {
	l.closeOnce.Do(func() {
		if l.eof != nil {
			l.closeErr = l.eof.Close()
		}
	})
	return l.closeErr
}

func (l *loggingIO) Config() cio.Config { return l.config }
func (l *loggingIO) Cancel()            { _ = l.CloseInput() }
func (*loggingIO) Wait()                {}
func (l *loggingIO) Close() error       { return l.CloseInput() }
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
		l := &loggingIO{config: cio.Config{Stdin: input, Stdout: u.String(), Stderr: u.String(), Terminal: tty}}
		if !stdin && !tty {
			// A real FIFO is required by Kata passfd I/O. Keep a writer until
			// Start, then close it to deliver EOF. The shim logger only keeps
			// interactive stdin.fifo open; this pipe needs no keeper on restart.
			path := filepath.Join(dir, "stdin-eof.fifo")
			if info, err := os.Lstat(path); err == nil {
				if info.Mode()&os.ModeNamedPipe == 0 {
					return nil, fmt.Errorf("EOF stdin path is not a FIFO")
				}
			} else if os.IsNotExist(err) {
				if err := syscall.Mkfifo(path, 0600); err != nil {
					return nil, err
				}
			} else {
				return nil, err
			}
			f, err := os.OpenFile(path, os.O_RDWR|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
			if err != nil {
				return nil, err
			}
			l.eof = f
			l.config.Stdin = path
		}
		return l, nil
	}, nil
}

// Keep the FIFO writer alive through passfd initialization. An immediately
// empty reader can close it before Kata opens its peer, making Start fail.
type execInputEOF struct {
	done chan struct{}
	once sync.Once
}

func newExecInputEOF() *execInputEOF             { return &execInputEOF{done: make(chan struct{})} }
func (e *execInputEOF) Read([]byte) (int, error) { <-e.done; return 0, io.EOF }
func (e *execInputEOF) Close()                   { e.once.Do(func() { close(e.done) }) }
