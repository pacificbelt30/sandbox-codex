package cmd

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"testing"
)

// TestResolveContainerUser_Empty verifies that an empty mode returns "" (image default).
func TestResolveContainerUser_Empty(t *testing.T) {
	got, err := resolveContainerUser("", "/tmp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("got %q; want \"\"", got)
	}
}

// TestResolveContainerUser_Current verifies that "current" returns the running
// process's uid:gid.
func TestResolveContainerUser_Current(t *testing.T) {
	uid := syscall.Getuid()
	gid := syscall.Getgid()
	want := fmt.Sprintf("%d:%d", uid, gid)

	got, err := resolveContainerUser("current", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("got %q; want %q", got, want)
	}
}

// TestResolveContainerUser_Dir verifies that "dir" returns the uid:gid of the
// directory owner.
func TestResolveContainerUser_Dir(t *testing.T) {
	dir := t.TempDir()

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat tempdir: %v", err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Skip("cannot cast to *syscall.Stat_t on this platform")
	}
	want := fmt.Sprintf("%d:%d", stat.Uid, stat.Gid)

	got, err := resolveContainerUser("dir", dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("got %q; want %q", got, want)
	}
}

// TestResolveContainerUser_Dir_NotExist verifies that "dir" with a missing path
// returns an error.
func TestResolveContainerUser_Dir_NotExist(t *testing.T) {
	_, err := resolveContainerUser("dir", "/nonexistent-path-codex-dock-test")
	if err == nil {
		t.Error("expected error for non-existent directory, got nil")
	}
}

// TestResolveContainerUser_Explicit verifies that an explicit uid or uid:gid is
// passed through unchanged.
func TestResolveContainerUser_Explicit(t *testing.T) {
	cases := []string{"1000", "1000:1000", "500:500", "0"}
	for _, tc := range cases {
		got, err := resolveContainerUser(tc, "")
		if err != nil {
			t.Errorf("resolveContainerUser(%q): unexpected error: %v", tc, err)
			continue
		}
		if got != tc {
			t.Errorf("resolveContainerUser(%q) = %q; want %q", tc, got, tc)
		}
	}
}

// TestResolveContainerUser_Current_Format verifies the output has "uid:gid" shape.
func TestResolveContainerUser_Current_Format(t *testing.T) {
	got, err := resolveContainerUser("current", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	parts := strings.Split(got, ":")
	if len(parts) != 2 {
		t.Errorf("expected uid:gid format, got %q", got)
	}
	for _, p := range parts {
		if p == "" {
			t.Errorf("empty part in uid:gid %q", got)
		}
	}
}

// TestResolveProjectDir_Dot verifies that "." resolves to the working directory.
func TestResolveProjectDir_Dot(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	got, err := resolveProjectDir(".")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != wd {
		t.Errorf("got %q; want %q", got, wd)
	}
}

// TestResolveProjectDir_Absolute verifies that an absolute path is returned as-is.
func TestResolveProjectDir_Absolute(t *testing.T) {
	got, err := resolveProjectDir("/tmp/myproject")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/tmp/myproject" {
		t.Errorf("got %q; want /tmp/myproject", got)
	}
}
