//go:build linux

package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// Kata's primary RPC terminal path can lose output and never report exit.
// A FIFO selects passfd instead. The shim keeps its own FIFO reader open,
// retaining buffered output while the node restarts; recovery resumes draining.
type terminalIO struct {
	*loggingIO
	stop chan struct{}
	done chan struct{}
	once sync.Once
}

func openTerminalIO(dir string, base *loggingIO) (*terminalIO, error) {
	path := filepath.Join(dir, "terminal.fifo")
	if err := syscall.Mkfifo(path, 0600); err != nil && !os.IsExist(err) {
		return nil, err
	}
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_NONBLOCK|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	var st unix.Stat_t
	if err = unix.Fstat(fd, &st); err != nil || st.Mode&unix.S_IFMT != unix.S_IFIFO {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("terminal output is not a FIFO")
	}
	log, err := os.OpenFile(filepath.Join(dir, "console.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	t := &terminalIO{loggingIO: base, stop: make(chan struct{}), done: make(chan struct{})}
	t.config.Stdout, t.config.Stderr = path, ""
	go t.collect(fd, log)
	return t, nil
}

func (t *terminalIO) collect(fd int, log *os.File) {
	defer close(t.done)
	defer func() { _ = unix.Close(fd); _ = log.Sync(); _ = log.Close() }()
	buf := make([]byte, 32768)
	var drainDeadline time.Time
	for {
		select {
		case <-t.stop:
			if drainDeadline.IsZero() {
				drainDeadline = time.Now().Add(250 * time.Millisecond)
			}
			if time.Now().After(drainDeadline) {
				return
			}
		default:
		}
		n, err := unix.Read(fd, buf)
		if n > 0 {
			if _, writeErr := log.Write(buf[:n]); writeErr != nil {
				return
			}
			continue
		}
		if err != nil && err != unix.EAGAIN && err != unix.EINTR {
			return
		}
		select {
		case <-t.stop:
			return // Drain available bytes before stopping.
		default:
		}
		_, _ = unix.Poll([]unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}, 50)
	}
}

func (t *terminalIO) Cancel()      { t.once.Do(func() { close(t.stop) }); <-t.done; t.loggingIO.Cancel() }
func (t *terminalIO) Close() error { t.Cancel(); return t.loggingIO.Close() }
func (t *terminalIO) Wait()        { t.Cancel() }
