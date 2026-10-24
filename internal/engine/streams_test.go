//go:build linux

package engine

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestSeekTailAcrossChunks(t *testing.T) {
	data := strings.Repeat("a", 40000) + "\nsecond\nthird\n"
	path := filepath.Join(t.TempDir(), "log")
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		n    int
		want string
	}{{0, ""}, {1, "third\n"}, {2, "second\nthird\n"}, {3, data}, {100, data}} {
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		if err = seekTail(f, tt.n); err != nil {
			t.Fatal(err)
		}
		got, err := io.ReadAll(f)
		_ = f.Close()
		if err != nil || string(got) != tt.want {
			t.Fatalf("tail %d: got length %d, want %d: %v", tt.n, len(got), len(tt.want), err)
		}
	}
}

func TestExecStreamHalfClosePreservesOutputAndFullCloseCancels(t *testing.T) {
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: filepath.Join(t.TempDir(), "exec.sock"), Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	client, err := net.DialUnix("unix", nil, listener.Addr().(*net.UnixAddr))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	server, err := listener.AcceptUnix()
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	parent, cancelRequest := context.WithCancel(context.Background())
	defer cancelRequest()
	ctx, stop, err := execStreamContext(parent, server)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	if _, err := client.Write([]byte("input")); err != nil {
		t.Fatal(err)
	}
	if err := client.CloseWrite(); err != nil {
		t.Fatal(err)
	}
	input, err := io.ReadAll(server)
	if err != nil || string(input) != "input" {
		t.Fatalf("stdin EOF: %q %v", input, err)
	}
	cancelRequest() // net/http observes this same EOF and cancels its context.
	select {
	case <-ctx.Done():
		t.Fatal("stdin EOF canceled exec")
	case <-time.After(250 * time.Millisecond):
	}
	if _, err := server.Write([]byte("output after EOF")); err != nil {
		t.Fatal(err)
	}
	output := make([]byte, len("output after EOF"))
	if _, err := io.ReadFull(client, output); err != nil || string(output) != "output after EOF" {
		t.Fatalf("stdout after EOF: %q %v", output, err)
	}
	client.Close()
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("full disconnect did not cancel silent exec")
	}
}

// A restarted container has historical output, but an attach requesting only
// stdin must finish copying input without trying to replay that output.
func TestStdinOnlyAttachDrainsInputWithoutReplayingRestartLogs(t *testing.T) {
	b := &Backend{opts: Options{State: t.TempDir()}}
	rec := &record{ID: "fixture"}
	rec.Config.OpenStdin = true
	creator, err := b.logIOAt(b.path(rec.ID), rec.ID, false, true, 0)
	if err != nil {
		t.Fatal(err)
	}
	streams, err := creator(rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = streams.Close() }()
	if err = os.WriteFile(filepath.Join(b.path(rec.ID), "console.log"), []byte("old startup output\n"), 0600); err != nil {
		t.Fatal(err)
	}
	fifo, err := os.Open(streams.Config().Stdin)
	if err != nil {
		t.Fatal(err)
	}
	defer fifo.Close()
	if err = fifo.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	tracked := &inputOnlyAttachConn{Conn: server}
	done := make(chan struct{})
	go func() {
		b.attachIO(context.Background(), tracked, bufio.NewReader(server), rec, true, false)
		_ = server.Close()
		close(done)
	}()
	payload := append([]byte{0, 0, 1, 0}, bytes.Repeat([]byte("x"), 256)...)
	writeResult := make(chan error, 1)
	go func() {
		_, err := client.Write(payload[:4])
		if err == nil {
			_, err = client.Write(payload[4:])
		}
		_ = client.Close()
		writeResult <- err
	}()
	got := make([]byte, len(payload))
	if _, err = io.ReadFull(fifo, got); err != nil || !bytes.Equal(got, payload) {
		t.Fatal("stdin frame was lost or truncated", err)
	}
	if err = <-writeResult; err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("stdin-only attach did not finish after EOF")
	}
	if tracked.writes.Load() != 0 {
		t.Fatal("stdin-only attach replayed historical console output")
	}
}

type inputOnlyAttachConn struct {
	net.Conn
	writes atomic.Int32
}

func (c *inputOnlyAttachConn) Write([]byte) (int, error) { c.writes.Add(1); return 0, net.ErrClosed }

// Attaching after a short-lived process exits must not wait for its closed
// stdin reader: its final console output is still available.
func TestAttachReplaysFinalOutputWithoutLiveStdinReader(t *testing.T) {
	b := &Backend{opts: Options{State: t.TempDir()}}
	rec := &record{ID: "fixture"}
	rec.Config.OpenStdin = true
	dir := b.path(rec.ID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(dir, "stdin.fifo"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "console.log"), []byte("final output\n"), 0600); err != nil {
		t.Fatal(err)
	}
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	_ = client.SetReadDeadline(time.Now().Add(time.Second))
	go func() {
		b.attachIO(context.Background(), server, strings.NewReader(""), rec, true, true)
		_ = server.Close()
	}()
	output, err := io.ReadAll(client)
	if err != nil || string(output) != "final output\n" {
		t.Fatalf("finished attach: %q %v", output, err)
	}
}
