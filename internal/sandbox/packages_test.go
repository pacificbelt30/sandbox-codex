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
		// Explicit prefix cases
		{"apt:libssl-dev", "apt", "libssl-dev"},
		{"pip:pwntools", "pip", "pwntools"},
		{"npm:typescript", "npm", "typescript"},
		{"APT:curl", "apt", "curl"},
		{"apt:python3-scipy", "apt", "python3-scipy"},
		// Auto-detection cases (F-PKG-05)
		{"pwntools", "apt", "pwntools"},   // no prefix → apt (default)
		{"golang", "apt", "golang"},       // no prefix → apt (default)
		{"@types/node", "npm", "@types/node"}, // @-scoped → npm
		{"requests>=2.0", "pip", "requests>=2.0"}, // PEP 508 → pip
		{"flask==2.3.0", "pip", "flask==2.3.0"},   // PEP 508 → pip
		{"numpy~=1.26", "pip", "numpy~=1.26"},     // PEP 508 → pip
		{"mylib!=1.0", "pip", "mylib!=1.0"},       // PEP 508 → pip
		{"scipy<=1.9", "pip", "scipy<=1.9"},       // PEP 508 → pip
		{"@angular/core", "npm", "@angular/core"},  // @-scoped → npm
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

// TestDetectManager exercises the auto-detection logic directly.
func TestDetectManager(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"curl", "apt"},
		{"libssl-dev", "apt"},
		{"git", "apt"},
		{"@types/node", "npm"},
		{"@angular/core", "npm"},
		{"requests==2.28.0", "pip"},
		{"flask>=2.0", "pip"},
		{"numpy<=1.26", "pip"},
		{"mylib~=1.0", "pip"},
		{"badlib!=2.0", "pip"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectManager(tt.name)
			if got != tt.want {
				t.Errorf("detectManager(%q) = %q; want %q", tt.name, got, tt.want)
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

	// "pwntools" has no prefix → auto-detected as apt (F-PKG-05)
	want := []Package{
		{Manager: "apt", Name: "pwntools"},
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

func TestLoadPackageFile_AutoDetect(t *testing.T) {
	content := `
@types/node
requests>=2.0
libssl-dev
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
		{Manager: "npm", Name: "@types/node"},
		{Manager: "pip", Name: "requests>=2.0"},
		{Manager: "apt", Name: "libssl-dev"},
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
			{Manager: "auto", Name: "make"}, // legacy "auto" still falls through to apt
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

	t.Run("auto-detected npm", func(t *testing.T) {
		pkgs := []Package{{Manager: "npm", Name: "@types/node"}}
		script := BuildInstallScript(pkgs)
		if !strings.Contains(script, "npm install") {
			t.Error("expected npm install for @types/node")
		}
		if !strings.Contains(script, "@types/node") {
			t.Error("expected @types/node in script")
		}
	})

	t.Run("auto-detected pip", func(t *testing.T) {
		pkgs := []Package{{Manager: "pip", Name: "requests>=2.0"}}
		script := BuildInstallScript(pkgs)
		if !strings.Contains(script, "pip3 install") {
			t.Error("expected pip3 install for requests>=2.0")
		}
		if !strings.Contains(script, "requests>=2.0") {
			t.Error("expected requests>=2.0 in script")
		}
	})
}
