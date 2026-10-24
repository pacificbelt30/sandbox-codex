//go:build linux

package engine

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestTerminalFIFORecoversBufferedOutputAndAppendsHistory(t *testing.T) {
	dir := t.TempDir()
	first, err := openTerminalIO(dir, &loggingIO{})
	if err != nil {
		t.Fatal(err)
	}
	path := first.Config().Stdout
	if !filepath.IsAbs(path) || first.Config().Stderr != "" {
		t.Fatal("not a passfd terminal FIFO")
	}
	// Kata retains this read descriptor while the node is absent.
	keeper, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = keeper.Close() }()
	writer, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = writer.Close() }()
	prefix := strings.Repeat("first\n", 10000)
	if _, err := writer.WriteString(prefix); err != nil {
		t.Fatal(err)
	}
	first.Cancel()
	if _, err := writer.WriteString("during-restart\n"); err != nil {
		t.Fatal(err)
	}
	second, err := openTerminalIO(dir, &loggingIO{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.WriteString("after-restart\n"); err != nil {
		t.Fatal(err)
	}
	second.Wait()
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "console.log"))
	if err != nil || string(data) != prefix+"during-restart\nafter-restart\n" {
		t.Fatalf("output lost across collector recovery: %d bytes %v", len(data), err)
	}
}

func TestTerminalOutputRejectsUnexpectedFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "terminal.fifo"), []byte("owned"), 0600); err != nil {
		t.Fatal(err)
	}
	if output, err := openTerminalIO(dir, &loggingIO{}); err == nil {
		output.Cancel()
		t.Fatal("adopted an ordinary file")
	}
}
