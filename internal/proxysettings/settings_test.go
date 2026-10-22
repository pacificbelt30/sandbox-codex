package proxysettings

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDefaultsDenyAndPreservesExplicitPublicAccess(t *testing.T) {
	for _, tc := range []struct {
		name, content string
		missing       bool
		deny          bool
	}{
		{name: "no file", missing: true, deny: true},
		{name: "empty file", deny: true},
		{name: "no proxy section", content: "[run]\nimage = 'worker'\n", deny: true},
		{name: "no deny setting", content: "[proxy]\nforward_deny_ports = [25]\n", deny: true},
		{name: "explicit public access", content: "[proxy]\nforward_default_deny = false\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			if !tc.missing {
				if err := os.WriteFile(path, []byte(tc.content), 0600); err != nil {
					t.Fatal(err)
				}
			}
			s, rev, err := Load(path)
			if err != nil || s.DefaultDeny != tc.deny {
				t.Fatalf("loaded = %+v, %v", s, err)
			}
			if err := Save(path, rev, s); err != nil {
				t.Fatal(err)
			}
			saved, _, err := Load(path)
			if err != nil || saved.DefaultDeny != tc.deny {
				t.Fatalf("saved policy changed default deny: %+v, %v", saved, err)
			}
		})
	}
}

func TestSavePreservesOtherSettingsAndDetectsConflict(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	original := "# keep this\nsecret = 'do-not-render'\n[proxy]\nadmin_secret = 'also-preserve'\nforward_allow_domains = ['old.example']\n[run]\nimage = 'keep'\n"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	_, rev, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	s := Settings{AllowDomains: []string{"example.com"}, DenyPorts: []int{22}}
	if err := Save(path, rev, s); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	for _, text := range []string{"# keep this", "secret = 'do-not-render'", "admin_secret = 'also-preserve'", "image = 'keep'"} {
		if !strings.Contains(string(data), text) {
			t.Fatalf("lost %s", text)
		}
	}
	got, _, err := Load(path)
	if err != nil || len(got.AllowDomains) != 1 || got.AllowDomains[0] != "example.com" || len(got.DenyPorts) != 1 {
		t.Fatalf("%+v %v", got, err)
	}
	if err := Save(path, rev, s); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
}
func TestSaveRefusesSymlinkAndInvalidPolicy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.Symlink("missing", path); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, revision(nil), Settings{}); err == nil {
		t.Fatal("symlink accepted")
	}
	if err := (Settings{AllowPorts: []int{0}}).Validate(); err == nil {
		t.Fatal("bad port accepted")
	}
}
