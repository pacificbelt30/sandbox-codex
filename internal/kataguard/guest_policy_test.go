package kataguard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGuestPIDCapabilitiesAreBoundToPreparedImage(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "assets.lock.json")
	imageHash := strings.Repeat("a", 64)
	for _, tc := range []struct {
		version          int
		policy, hash     string
		supported, valid bool
	}{
		{1, "", imageHash, false, true}, {2, guestPIDPolicy, imageHash, true, true}, {2, guestPIDPolicy, strings.Repeat("b", 64), false, false}, {2, "unknown", imageHash, false, false}, {3, guestPIDPolicy, imageHash, false, false},
	} {
		data, err := json.Marshal(map[string]any{"schema_version": tc.version, "pids_limit": 512, "pid_policy": tc.policy, "files": map[string]string{"guest.image": tc.hash}})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		supported, err := readGuestPolicy(path, imageHash)
		if (err == nil) != tc.valid || supported != tc.supported {
			t.Fatalf("guest policy v%d: supported=%v err=%v", tc.version, supported, err)
		}
	}
}
