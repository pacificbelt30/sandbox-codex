//go:build linux

package engine

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestNoninteractiveLoggingInputDeliversEOFAndCanRestart(t *testing.T) {
	dir := t.TempDir()
	b := &Backend{opts: Options{Logger: "/fixture/logger"}}
	for i := 0; i < 2; i++ {
		creator, err := b.logIOAt(dir, "fixture", false, false, 0)
		if err != nil {
			t.Fatal(err)
		}
		stream, err := creator("fixture")
		if err != nil {
			t.Fatal(err)
		}
		l, ok := stream.(*loggingIO)
		if !ok {
			t.Fatal("missing controlled EOF source")
		}
		if filepath.Base(l.Config().Stdin) != "stdin-eof.fifo" {
			t.Fatal("interactive keeper would hold EOF open")
		}
		reader, err := os.OpenFile(l.Config().Stdin, os.O_RDONLY|syscall.O_NONBLOCK, 0)
		if err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { _, err := reader.Read(make([]byte, 1)); done <- err }()
		select {
		case err := <-done:
			t.Fatalf("EOF arrived before Start: %v", err)
		case <-time.After(20 * time.Millisecond):
		}
		if err := l.CloseInput(); err != nil {
			t.Fatal(err)
		}
		select {
		case err := <-done:
			if !errors.Is(err, io.EOF) {
				t.Fatalf("expected EOF, got %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("EOF was not delivered")
		}
		_ = reader.Close()
		l.Cancel()
		if err := l.Close(); err != nil {
			t.Fatal("EOF closure is not idempotent:", err)
		}
	}
}

func TestEOFInputDoesNotAdoptUnexpectedFilesOrInteractiveInput(t *testing.T) {
	dir := t.TempDir()
	b := &Backend{opts: Options{Logger: "/fixture/logger"}}
	if err := os.WriteFile(filepath.Join(dir, "stdin-eof.fifo"), []byte("owned-elsewhere"), 0600); err != nil {
		t.Fatal(err)
	}
	creator, err := b.logIOAt(dir, "fixture", false, false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := creator("fixture"); err == nil {
		t.Fatal("ordinary file adopted as stdin")
	}
	creator, err = b.logIOAt(dir, "fixture", false, true, 0)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := creator("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(stream.Config().Stdin) != "stdin.fifo" {
		t.Fatal("interactive input was replaced")
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
}
