package sandbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParsePackage(t *testing.T) {
	tests := []struct {
		spec        string
		wantManager string
		wantName    string
	}{
		{"apt:libssl-dev", "apt", "libssl-dev"},
		{"pip:pwntools", "pip", "pwntools"},
		{"npm:typescript", "npm", "typescript"},
		{"APT:curl", "apt", "curl"},
		{"pwntools", "auto", "pwntools"},
		{"golang", "auto", "golang"},
		{"apt:python3-scipy", "apt", "python3-scipy"},
	}
	for _, tt := range tests {
		t.Run(tt.spec, func(t *testing.T) {
			got := ParsePackage(tt.spec)
			if got.Manager != tt.wantManager {
				t.Errorf("Manager = %q; want %q", got.Manager, tt.wantManager)
			}
			if got.Name != tt.wantName {
				t.Errorf("Name = %q; want %q", got.Name, tt.wantName)
			}
		})
	}
}

func TestLoadPackageFile(t *testing.T) {
	content := `
# This is a comment
pwntools
apt:gdb
pip:requests

npm:typescript   # inline comment
`
	f, err := os.CreateTemp(t.TempDir(), "packages.dock")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()

	pkgs, err := LoadPackageFile(f.Name())
	if err != nil {
		t.Fatalf("LoadPackageFile: %v", err)
	}

	want := []Package{
		{Manager: "auto", Name: "pwntools"},
		{Manager: "apt", Name: "gdb"},
		{Manager: "pip", Name: "requests"},
		{Manager: "npm", Name: "typescript"},
	}
	if len(pkgs) != len(want) {
		t.Fatalf("got %d packages; want %d", len(pkgs), len(want))
	}
	for i, p := range pkgs {
		if p != want[i] {
			t.Errorf("pkg[%d] = %+v; want %+v", i, p, want[i])
		}
	}
}

func TestLoadPackageFile_NotFound(t *testing.T) {
	_, err := LoadPackageFile(filepath.Join(t.TempDir(), "nonexistent.dock"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadPackageFile_Empty(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "packages.dock")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("# only comments\n\n  \n")
	f.Close()

	pkgs, err := LoadPackageFile(f.Name())
	if err != nil {
		t.Fatalf("LoadPackageFile: %v", err)
	}
	if len(pkgs) != 0 {
		t.Errorf("expected 0 packages, got %d", len(pkgs))
	}
}

func TestBuildInstallScript(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		script := BuildInstallScript(nil)
		if script != "" {
			t.Errorf("expected empty script, got %q", script)
		}
	})

	t.Run("apt only", func(t *testing.T) {
		pkgs := []Package{{Manager: "apt", Name: "curl"}, {Manager: "apt", Name: "git"}}
		script := BuildInstallScript(pkgs)
		if !strings.Contains(script, "apt-get install") {
			t.Error("expected apt-get install in script")
		}
		if !strings.Contains(script, "curl") || !strings.Contains(script, "git") {
			t.Error("expected package names in script")
		}
		if strings.Contains(script, "pip3") || strings.Contains(script, "npm") {
			t.Error("unexpected pip/npm in apt-only script")
		}
	})

	t.Run("pip only", func(t *testing.T) {
		pkgs := []Package{{Manager: "pip", Name: "requests"}}
		script := BuildInstallScript(pkgs)
		if !strings.Contains(script, "pip3 install") {
			t.Error("expected pip3 install in script")
		}
	})

	t.Run("npm only", func(t *testing.T) {
		pkgs := []Package{{Manager: "npm", Name: "typescript"}}
		script := BuildInstallScript(pkgs)
		if !strings.Contains(script, "npm install") {
			t.Error("expected npm install in script")
		}
	})

	t.Run("mixed", func(t *testing.T) {
		pkgs := []Package{
			{Manager: "apt", Name: "libssl-dev"},
			{Manager: "pip", Name: "pwntools"},
			{Manager: "npm", Name: "typescript"},
			{Manager: "auto", Name: "make"},
		}
		script := BuildInstallScript(pkgs)
		if !strings.Contains(script, "apt-get install") {
			t.Error("expected apt-get install")
		}
		if !strings.Contains(script, "pip3 install") {
			t.Error("expected pip3 install")
		}
		if !strings.Contains(script, "npm install") {
			t.Error("expected npm install")
		}
		if !strings.HasPrefix(script, "set -e") {
			t.Error("expected script to start with 'set -e'")
		}
	})
}
