package kataguard

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOldBootstrapCannotAdvertiseTemporaryAttestation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "guard")
	for _, tc := range []struct {
		body      string
		supported bool
	}{
		{"exit 1", false}, {`printf '%s\n' '["temporary-storage-probe-v1"]'`, true}, {`printf '%s\n' '["other-capability"]'`, false}, {`printf '%s\n' 'malformed'`, false},
	} {
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"+tc.body+"\n"), 0700); err != nil {
			t.Fatal(err)
		}
		if got := launcherTemporaryStorage(path); got != tc.supported {
			t.Fatalf("bootstrap capabilities=%v, want %v", got, tc.supported)
		}
	}
}
