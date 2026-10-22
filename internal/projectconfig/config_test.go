package projectconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadOptionalOaisoConfigAndRejectUnmigratedSettings(t *testing.T) {
	const settings = "[run]\nagent = 'claude'\nread_only = true\nno_internet = true\n"
	for _, tc := range []struct {
		name              string
		canonical, legacy bool
	}{
		{name: "optional"},
		{name: "oaiso", canonical: true},
		{name: "old name", legacy: true},
		{name: "both names", canonical: true, legacy: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, present := range map[string]bool{"oaiso.toml": tc.canonical, "codex-dock.toml": tc.legacy} {
				if present {
					if err := os.WriteFile(filepath.Join(dir, name), []byte(settings), 0644); err != nil {
						t.Fatal(err)
					}
				}
			}
			cfg, err := Load(dir)
			if tc.legacy {
				if cfg != nil || err == nil || !strings.Contains(err.Error(), "oaiso.toml") || !strings.Contains(err.Error(), "codex-dock.toml") {
					t.Fatalf("expected migration error, got config=%+v error=%v", cfg, err)
				}
				data, readErr := os.ReadFile(filepath.Join(dir, "codex-dock.toml"))
				if readErr != nil || string(data) != settings {
					t.Fatal("old settings changed")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !tc.canonical {
				if cfg != nil {
					t.Fatal("a project config must remain optional")
				}
				return
			}
			if cfg == nil || cfg.Run.Agent != "claude" || cfg.Run.ReadOnly == nil || !*cfg.Run.ReadOnly || cfg.Run.NoInternet == nil || !*cfg.Run.NoInternet {
				t.Fatalf("new filename lost project restrictions: %+v", cfg)
			}
		})
	}
}

func TestRenameProjectConfigPreservesRestrictions(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "codex-dock.toml")
	if err := os.WriteFile(legacy, []byte("[run]\nread_only = true\nno_internet = true\ndeny_read = ['/workspace/private']\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil {
		t.Fatal("old filename was silently ignored")
	}
	if err := os.Rename(legacy, filepath.Join(dir, "oaiso.toml")); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg == nil || cfg.Run.ReadOnly == nil || !*cfg.Run.ReadOnly || cfg.Run.NoInternet == nil || !*cfg.Run.NoInternet || len(cfg.Run.DenyRead) != 1 || cfg.Run.DenyRead[0] != "/workspace/private" {
		t.Fatalf("restrictions lost after rename: %+v", cfg)
	}
}

func TestLoadDetectsDanglingLegacySymlink(t *testing.T) {
	dir := t.TempDir()
	if err := os.Symlink("missing-settings", filepath.Join(dir, "codex-dock.toml")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "obsolete project config") {
		t.Fatalf("old symlink was silently ignored: %v", err)
	}
}
