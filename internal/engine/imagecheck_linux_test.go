//go:build linux

package engine

import (
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"os"
	"path/filepath"
	"testing"
)

func TestImageCheckStaysInsideRootAndDoesNotRunCommands(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "usr/bin"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/usr/bin", filepath.Join(root, "bin")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"bash", "curl", "python3", "git", "codex"} {
		// Would fail/create a file if a probe tried to execute it.
		if err := os.WriteFile(filepath.Join(root, "usr/bin", name), []byte("#!/bin/sh\nexit 91\n"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	image := v1.Image{Platform: v1.Platform{OS: "linux", Architecture: "amd64"}, Config: v1.ImageConfig{Entrypoint: []string{"/bin/bash"}}}
	report := inspectImageRoot(root, image, "codex")
	if err := report.Err(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "usr/bin/python3")); err != nil {
		t.Fatal(err)
	}
	// An absolute symlink must not resolve to the host's Python.
	if err := os.Symlink("/usr/bin/python3", filepath.Join(root, "usr/bin/python3")); err != nil {
		t.Fatal(err)
	}
	if err := inspectImageRoot(root, image, "codex").Err(); err == nil {
		t.Fatal("host executable escaped image root")
	}
	image.OS = "windows"
	if err := inspectImageRoot(root, image, "shell").Err(); err == nil {
		t.Fatal("Windows image accepted")
	}
}
