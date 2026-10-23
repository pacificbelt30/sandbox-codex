package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUpgradeRefusesKnownIncompatibleEncryptedDowngrade(t *testing.T) {
	dir := t.TempDir()
	profile := filepath.Join(dir, "work")
	if err := os.Mkdir(profile, 0700); err != nil {
		t.Fatal(err)
	}
	if err := rejectUnsupportedProfileDowngrade("v0.0.2", dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profile, "work.vault"), []byte("ciphertext"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tag := range []string{"v0.0.1", "v0.0.2", "v0.0.2-test"} {
		if err := rejectUnsupportedProfileDowngrade(tag, dir); err == nil {
			t.Fatal("unsupported encrypted downgrade allowed", tag)
		}
	}
	if err := rejectUnsupportedProfileDowngrade("v0.1.0", dir); err != nil {
		t.Fatal(err)
	}
}
