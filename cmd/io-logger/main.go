// io-logger is the pinned containerd binary-I/O helper. The shim owns its
// lifetime, so node-service restarts cannot close a VM's console or stdin.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"syscall"
)

func run() error {
	var dir, id string
	flag.StringVar(&dir, "state", "", "Private container record directory")
	flag.StringVar(&id, "id", "", "Container identity")
	flag.Parse()
	if os.Geteuid() != 0 || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(id) || !filepath.IsAbs(dir) || filepath.Clean(dir) != dir || filepath.Base(dir) != id {
		return fmt.Errorf("invalid logger identity")
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != 0 || info.Mode().Perm()&0077 != 0 || !info.IsDir() {
		return fmt.Errorf("untrusted logger state directory")
	}
	log, err := os.OpenFile(filepath.Join(dir, "console.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer func() { _ = log.Close() }()
	// Keep a writer open while a terminal detaches or the node restarts. No
	// bytes are read here: stdin belongs exclusively to the guest's reader.
	inputPath := filepath.Join(dir, "stdin.fifo")
	if info, err := os.Lstat(inputPath); err == nil {
		if info.Mode()&os.ModeNamedPipe == 0 {
			return fmt.Errorf("stdin is not a FIFO")
		}
		fd, err := syscall.Open(inputPath, syscall.O_RDWR|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
		if err != nil {
			return err
		}
		defer func() { _ = syscall.Close(fd) }()
	} else if !os.IsNotExist(err) {
		return err
	}
	stdout := os.NewFile(3, "stdout")
	stderr := os.NewFile(4, "stderr")
	ready := os.NewFile(5, "ready")
	if stdout == nil || stderr == nil || ready == nil {
		return fmt.Errorf("missing binary-I/O descriptors")
	}
	if _, err = ready.Write([]byte{1}); err != nil {
		return err
	}
	_ = ready.Close()
	var wg sync.WaitGroup
	var lock sync.Mutex
	var first error
	copyStream := func(input *os.File) {
		defer wg.Done()
		defer func() { _ = input.Close() }()
		_, err := io.Copy(log, input)
		if err != nil {
			lock.Lock()
			if first == nil {
				first = err
			}
			lock.Unlock()
		}
	}
	wg.Add(2)
	go copyStream(stdout)
	go copyStream(stderr)
	wg.Wait()
	if first != nil {
		return first
	}
	return log.Sync()
}
func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "io-logger:", err)
		os.Exit(1)
	}
}
