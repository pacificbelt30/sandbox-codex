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

func TestGoInstallKeepsCanonicalAndLegacyCommands(t *testing.T) {
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
	for _, name := range []string{"oaiso", "cxd"} {
		if target, err := os.Readlink(filepath.Join(bin, name)); err != nil || target != "codex-dock" {
			t.Fatalf("%s alias = %q, %v", name, target, err)
		}
	}
	for _, aliasName := range []string{"oaiso", "cxd"} {
		alias := filepath.Join(bin, aliasName)
		if err := os.Remove(alias); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(alias, []byte("unrelated"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := ensureGoInstalledAlias(context.Background(), goBin); err == nil || !strings.Contains(err.Error(), "leave it untouched") {
			t.Fatalf("unrelated %s was accepted: %v", aliasName, err)
		}
		if data, err := os.ReadFile(alias); err != nil || string(data) != "unrelated" {
			t.Fatalf("unrelated %s changed: %q, %v", aliasName, data, err)
		}
		if err := os.Remove(alias); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("elsewhere", alias); err != nil {
			t.Fatal(err)
		}
		if err := ensureGoInstalledAlias(context.Background(), goBin); err == nil || !strings.Contains(err.Error(), "leave it untouched") {
			t.Fatal("unrelated symlink accepted")
		}
		if target, err := os.Readlink(alias); err != nil || target != "elsewhere" {
			t.Fatal("unrelated symlink modified")
		}
		if err := os.Remove(alias); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("codex-dock", alias); err != nil {
			t.Fatal(err)
		}
	}
}
