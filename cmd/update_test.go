package cmd

import (
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

func TestExecuteUpdateInstallsCanonicalEntryPoint(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fixture uses a shell script")
	}
	dir := t.TempDir()
	log := filepath.Join(dir, "calls")
	t.Setenv("OAISO_UPDATE_TEST_LOG", log)
	t.Setenv("PATH", dir)
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$OAISO_UPDATE_TEST_LOG\"\n"
	if err := os.WriteFile(filepath.Join(dir, "go"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	if err := executeUpdate(t.Context()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(data)); got != "install\ngithub.com/pacificbelt30/codex-dock/cmd/oaiso@latest" {
		t.Fatalf("unexpected update command: %q", got)
	}
}
