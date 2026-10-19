package proxysettings

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
