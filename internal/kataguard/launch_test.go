package kataguard

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type testRequestFinder struct {
	id, key, fingerprint string
	found                bool
}

func (f *testRequestFinder) FindRequest(context.Context, string) (string, string, string, bool, error) {
	return f.id, f.key, f.fingerprint, f.found, nil
}

func digest(s string) string {
	b := sha256.Sum256([]byte(s))
	return hex.EncodeToString(b[:])
}

func trustedTestState(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, path := range []string{dir, filepath.Dir(dir)} {
		if err := os.Chmod(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestLaunchClaimPersistsAndCannotBeStolen(t *testing.T) {
	state := trustedTestState(t)
	g := &Guard{config: Config{StateDir: state}}
	key, fingerprint := digest("product/job/1"), digest("options")
	finder := &testRequestFinder{}
	first, err := g.claimLaunch(context.Background(), finder, key, fingerprint)
	if err != nil || first.State != "pending" || first.Lease == "" {
		t.Fatalf("first claim = %+v, %v", first, err)
	}
	// A different product process sees the pending result but receives no lease.
	restarted := &Guard{config: Config{StateDir: state}}
	second, err := restarted.claimLaunch(context.Background(), finder, key, fingerprint)
	if err != nil || second.State != "pending" || second.Lease != "" {
		t.Fatalf("repeated claim = %+v, %v", second, err)
	}
	if _, err := restarted.claimLaunch(context.Background(), finder, key, digest("changed options")); err == nil {
		t.Fatal("changed options reused the request ID")
	}
	finder.id, finder.key, finder.fingerprint, finder.found = digest("worker"), key, fingerprint, true
	// Container creation precedes verification, hooks and the launcher's finish.
	// Seeing that container must not let a concurrent retry complete its lease.
	second, err = restarted.claimLaunch(context.Background(), finder, key, fingerprint)
	if err != nil || second.State != "pending" || second.Lease != "" {
		t.Fatalf("retry during startup = %+v, %v", second, err)
	}
	if _, err := restarted.finishLaunch(context.Background(), finder, key, "wrong-lease", finder.id); err == nil {
		t.Fatal("another claim completed the launch")
	}
	completed, err := restarted.finishLaunch(context.Background(), finder, key, first.Lease, finder.id)
	if err != nil || completed.State != "complete" || completed.WorkerID != finder.id {
		t.Fatalf("completed launch = %+v, %v", completed, err)
	}
	again, err := g.claimLaunch(context.Background(), finder, key, fingerprint)
	if err != nil || again.State != "complete" || again.WorkerID != finder.id {
		t.Fatalf("claim after lost response = %+v, %v", again, err)
	}
}

func TestLaunchClaimRecoversWorkerAfterExpiredLease(t *testing.T) {
	g := &Guard{config: Config{StateDir: trustedTestState(t)}}
	key, fingerprint := digest("product/job/2"), digest("options")
	finder := &testRequestFinder{}
	if _, err := g.claimLaunch(context.Background(), finder, key, fingerprint); err != nil {
		t.Fatal(err)
	}
	r, err := g.readLaunch(key)
	if err != nil {
		t.Fatal(err)
	}
	r.UpdatedAt = time.Now().Add(-launchLeasePeriod - time.Second)
	if err := g.writeLaunch(r); err != nil {
		t.Fatal(err)
	}
	finder.id, finder.key, finder.fingerprint, finder.found = digest("worker-2"), key, fingerprint, true
	recovered, err := g.claimLaunch(context.Background(), finder, key, fingerprint)
	if err != nil || recovered.State != "complete" || recovered.WorkerID != finder.id {
		t.Fatalf("recovery = %+v, %v", recovered, err)
	}
}
