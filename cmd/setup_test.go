package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupPythonFixture(t *testing.T) string {
	t.Helper()
	python := filepath.Join(t.TempDir(), "python with spaces")
	if err := os.WriteFile(python, []byte("#!/bin/sh\n[ \"$1\" = -E ] && [ \"$2\" = -s ] && [ \"$3\" = -c ]\n"), 0755); err != nil {
		t.Fatal(err)
	}
	return python
}

func TestSetupPythonUsesUVWithoutProjectOrDownloads(t *testing.T) {
	python := setupPythonFixture(t)
	dir := t.TempDir()
	log := filepath.Join(dir, "uv-args")
	uv := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$SETUP_TEST_UV_LOG\"\nprintf '%s\\n' \"$SETUP_TEST_PYTHON\"\n"
	if err := os.WriteFile(filepath.Join(dir, "uv"), []byte(uv), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("SETUP_TEST_PYTHON", python)
	t.Setenv("SETUP_TEST_UV_LOG", log)
	got, err := setupPython(context.Background(), "", true)
	if err != nil || got != python {
		t.Fatalf("uv Python = %q, %v", got, err)
	}
	args, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"python", "find", "--no-project", "--no-config", "--managed-python", "--system", "--no-python-downloads", "--offline", "3.12"} {
		if !strings.Contains("\n"+string(args), "\n"+required+"\n") {
			t.Fatalf("uv resolution lacks %s: %s", required, args)
		}
	}
}

func TestSetupPythonExplicitPathWorksWithoutUV(t *testing.T) {
	python := setupPythonFixture(t)
	t.Setenv("PATH", t.TempDir()) // Simulate sudo's different PATH and HOME.
	t.Setenv("HOME", t.TempDir())
	got, err := setupPython(context.Background(), python, true)
	if err != nil || got != python {
		t.Fatalf("explicit Python = %q, %v", got, err)
	}
	if _, err := setupPython(context.Background(), "python3", true); err == nil {
		t.Fatal("relative interpreter accepted for privileged setup")
	}
}

func TestSetupPythonDoesNotFallBackToSystemPython(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "python3"), []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	if _, err := setupPython(context.Background(), "", false); err == nil || !strings.Contains(err.Error(), "uv-managed Python") {
		t.Fatalf("missing uv must fail with guidance: %v", err)
	}
	broken := filepath.Join(dir, "broken-python")
	if err := os.WriteFile(broken, []byte("#!/bin/sh\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := setupPython(context.Background(), broken, true); err == nil {
		t.Fatal("unusable interpreter accepted")
	}
}

func TestSetupBundleBesideExecutable(t *testing.T) {
	dir := t.TempDir()
	cli := filepath.Join(dir, "oaiso")
	for _, path := range []string{cli, filepath.Join(dir, "bundle.json")} {
		if err := os.WriteFile(path, []byte("test"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(t.TempDir(), "oaiso")
	if err := os.Symlink(cli, link); err != nil {
		t.Fatal(err)
	}
	got, err := setupBundleBeside(link)
	if err != nil || got != dir {
		t.Fatalf("setupBundleBeside(%s) = %q, %v; want %q", link, got, err, dir)
	}
	if err := os.Remove(filepath.Join(dir, "bundle.json")); err != nil {
		t.Fatal(err)
	}
	_, err = setupBundleBeside(link)
	if err == nil || !strings.Contains(err.Error(), "--bundle PATH") {
		t.Fatalf("expected explicit bundle guidance, got %v", err)
	}
}

func TestSetupOperatorRequiresExplicitRootGrant(t *testing.T) {
	if got, err := setupOperator("alice", true); err != nil || got != "alice" {
		t.Fatalf("explicit operator = %q, %v", got, err)
	}
	if _, err := setupOperator("", true); err == nil || !strings.Contains(err.Error(), "--operator USER") {
		t.Fatalf("expected explicit root grant guidance, got %v", err)
	}
}
