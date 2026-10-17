//go:build linux

package engine

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
