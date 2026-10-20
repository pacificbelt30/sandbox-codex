//go:build linux

package engine

import (
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
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
