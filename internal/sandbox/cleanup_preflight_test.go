package sandbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunRejectsUnusableCleanupStateBeforeCreatingResources(t *testing.T) {
	for _, scenario := range []string{"writable-parent", "symlink", "unwritable"} {
		t.Run(scenario, func(t *testing.T) {
			if scenario == "unwritable" && os.Geteuid() == 0 {
				t.Skip("root can write despite directory mode")
			}
			f, m := newEngineFixture(t)
			root := t.TempDir()
			switch scenario {
			case "writable-parent":
				if err := os.Chmod(root, 0770); err != nil {
					t.Fatal(err)
				}
				m.stateDir = filepath.Join(root, "state")
			case "symlink":
				m.stateDir = filepath.Join(t.TempDir(), "linked")
				if err := os.Symlink(root, m.stateDir); err != nil {
					t.Fatal(err)
				}
			case "unwritable":
				m.stateDir = root
				if err := os.Chmod(root, 0500); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(root, 0700) })
			}
			t.Setenv("CODEX_DOCK_STATE_DIR", m.stateDir)
			proxy := &fakeProxy{endpoint: "http://auth:18080"}
			m.proxy = proxy
			id, err := m.Run(RunOptions{Name: "rejected-state", ProjectDir: t.TempDir(), Detach: true})
			if err == nil {
				t.Fatal("unusable cleanup state was accepted")
			}
			if !strings.Contains(err.Error(), "cleanup") && !strings.Contains(err.Error(), "permission denied") {
				t.Fatalf("unexpected rejection: %v", err)
			}
			if id != "" || f.front != nil || f.worker != nil || f.starts != 0 || f.removed != 0 || len(proxy.issued) != 0 {
				t.Fatalf("rejection left side effects: id=%q network=%v worker=%v starts=%d removals=%d issued=%v", id, f.front != nil, f.worker != nil, f.starts, f.removed, proxy.issued)
			}
		})
	}
}
