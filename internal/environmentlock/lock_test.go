package environmentlock

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLockDetectsSourceChangesAndNeverOverwrites(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "Dockerfile")
	if err := os.WriteFile(source, []byte("FROM base"), 0600); err != nil {
		t.Fatal(err)
	}
	l, err := New(root, "worker:test", "sha256:"+strings.Repeat("a", 64), []string{"Dockerfile"})
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "lock.json")
	if err = l.Write(file); err != nil {
		t.Fatal(err)
	}
	if err = l.Write(file); err == nil {
		t.Fatal("overwrote lock")
	}
	if _, err = Load(file, root, "worker:test"); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(source, []byte("FROM changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = Load(file, root, "worker:test"); err == nil {
		t.Fatal("changed source accepted")
	}
	if _, err = New(root, "image", l.ImageID, []string{"../outside"}); err == nil {
		t.Fatal("outside source accepted")
	}
}

func TestLockNeverAcceptsAnUnpinnedImage(t *testing.T) {
	if _, err := New(t.TempDir(), "worker", "", nil); err == nil {
		t.Fatal("lock accepted missing digest")
	}
	path := filepath.Join(t.TempDir(), "lock.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"image":"worker","image_id":""}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path, t.TempDir(), "worker"); err == nil {
		t.Fatal("loaded lock with no digest")
	}
}
