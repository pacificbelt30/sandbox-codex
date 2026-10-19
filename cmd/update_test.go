package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestExecuteUpdate_GoNotFound(t *testing.T) {
	t.Setenv("PATH", "")
	if err := executeUpdate(t.Context()); err == nil {
		t.Fatal("expected error when go is not found in PATH")
	}
}

func TestGoInstallKeepsCxdCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fixture uses a shell script")
	}
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "codex-dock"), []byte("fixture"), 0755); err != nil {
		t.Fatal(err)
	}
	goBin := filepath.Join(root, "fake-go")
	script := fmt.Sprintf("#!/bin/sh\nprintf '\\n%%s\\n' %q\n", root)
	if err := os.WriteFile(goBin, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := ensureGoInstalledAlias(context.Background(), goBin); err != nil {
			t.Fatal(err)
		}
	}
	alias := filepath.Join(bin, "cxd")
	if target, err := os.Readlink(alias); err != nil || target != "codex-dock" {
		t.Fatalf("cxd alias = %q, %v", target, err)
	}
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(alias, []byte("unrelated"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := ensureGoInstalledAlias(context.Background(), goBin); err == nil || !strings.Contains(err.Error(), "leave it untouched") {
		t.Fatalf("unrelated cxd was accepted: %v", err)
	}
	if data, err := os.ReadFile(alias); err != nil || string(data) != "unrelated" {
		t.Fatalf("unrelated cxd changed: %q, %v", data, err)
	}
}
