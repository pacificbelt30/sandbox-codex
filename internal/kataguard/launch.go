package kataguard

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

const launchLeasePeriod = 5 * time.Minute

var requestDigest = regexp.MustCompile(`^[0-9a-f]{64}$`)

var ErrLaunchConflict = errors.New("launch request conflict")

// LaunchRecord is the durable result of an externally identified launch.
// Keys and fingerprints are hashes; raw job IDs and options stay in the caller.
type LaunchRecord struct {
	Version     int       `json:"version"`
	Key         string    `json:"key"`
	Fingerprint string    `json:"fingerprint"`
	Name        string    `json:"name"`
	State       string    `json:"state"`
	Lease       string    `json:"lease,omitempty"`
	WorkerID    string    `json:"worker_id,omitempty"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type requestFinder interface {
	FindRequest(context.Context, string) (id, key, fingerprint string, found bool, err error)
}

func requestName(key string) string { return "cxd-" + key[:32] }

func RequestAllocation(key string) string {
	sum := sha256.Sum256([]byte("allocation:" + key))
	return hex.EncodeToString(sum[:16])
}

func (g *Guard) launchDir() (string, error) {
	dir := filepath.Join(g.config.StateDir, "launches")
	if err := os.Mkdir(dir, 0700); err != nil && !os.IsExist(err) {
		return "", err
	}
	return dir, trustedPath(dir, true)
}

func (g *Guard) readLaunch(key string) (LaunchRecord, error) {
	var r LaunchRecord
	if !requestDigest.MatchString(key) {
		return r, fmt.Errorf("invalid request key")
	}
	dir, err := g.launchDir()
	if err != nil {
		return r, err
	}
	path := filepath.Join(dir, key+".json")
	if _, err := os.Lstat(path); err != nil {
		return r, err
	}
	if err := trustedPath(path, false); err != nil {
		return r, err
	}
	f, err := os.Open(path)
	if err != nil {
		return r, err
	}
	defer f.Close()
	dec := json.NewDecoder(io.LimitReader(f, 4096))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		return r, err
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		return r, fmt.Errorf("trailing launch record")
	}
	if r.Version != 1 || r.Key != key || !requestDigest.MatchString(r.Fingerprint) || r.Name != requestName(key) || r.UpdatedAt.IsZero() {
		return r, fmt.Errorf("invalid launch record")
	}
	switch r.State {
	case "pending", "complete", "failed", "cleanup-pending":
	default:
		return r, fmt.Errorf("invalid launch state")
	}
	return r, nil
}

func (g *Guard) writeLaunch(r LaunchRecord) error {
	dir, err := g.launchDir()
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".launch-")
	if err != nil {
		return err
	}
	defer func() { _ = f.Close(); _ = os.Remove(f.Name()) }()
	if err := json.NewEncoder(f).Encode(r); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), filepath.Join(dir, r.Key+".json")); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

func (g *Guard) claimLaunch(ctx context.Context, finder requestFinder, key, fingerprint string) (LaunchRecord, error) {
	if !requestDigest.MatchString(key) || !requestDigest.MatchString(fingerprint) {
		return LaunchRecord{}, fmt.Errorf("invalid launch request digest")
	}
	if err := ctx.Err(); err != nil {
		return LaunchRecord{}, err
	}
	r, err := g.readLaunch(key)
	fresh := os.IsNotExist(err)
	if fresh {
		r = LaunchRecord{Version: 1, Key: key, Fingerprint: fingerprint, Name: requestName(key), State: "failed"}
	} else if err != nil {
		return LaunchRecord{}, err
	} else if r.Fingerprint != fingerprint {
		return LaunchRecord{}, fmt.Errorf("%w: request ID was already used with different launch options", ErrLaunchConflict)
	}
	if r.State == "complete" || r.State == "cleanup-pending" {
		return r, nil
	}
	id, foundKey, foundFingerprint, found, err := finder.FindRequest(ctx, r.Name)
	if err != nil {
		return LaunchRecord{}, err
	}
	if found {
		if foundKey != key || foundFingerprint != fingerprint {
			return LaunchRecord{}, fmt.Errorf("%w: request worker name is occupied by another worker", ErrLaunchConflict)
		}
		r.State, r.WorkerID, r.Lease, r.UpdatedAt = "complete", id, "", time.Now().UTC()
		return r, g.writeLaunch(r)
	}
	if r.State == "pending" && time.Since(r.UpdatedAt) < launchLeasePeriod {
		r.Lease = "" // A second caller must never inherit the active claim.
		return r, nil
	}
	if !fresh && (r.State == "pending" || r.State == "failed") {
		inventory, err := g.Inventory(ctx)
		if err != nil {
			return LaunchRecord{}, err
		}
		if !inventory.EngineAvailable {
			return LaunchRecord{}, fmt.Errorf("engine unavailable; cannot reconcile launch")
		}
		for _, vm := range inventory.VMs {
			if vm.Allocation == RequestAllocation(key) {
				r.Lease = ""
				return r, nil // A live or uncertain reservation must be recovered first.
			}
		}
	}
	lease := make([]byte, 16)
	if _, err := rand.Read(lease); err != nil {
		return LaunchRecord{}, err
	}
	r.State, r.Lease, r.WorkerID, r.UpdatedAt = "pending", hex.EncodeToString(lease), "", time.Now().UTC()
	return r, g.writeLaunch(r)
}

func (g *Guard) finishLaunch(ctx context.Context, finder requestFinder, key, lease, workerID string) (LaunchRecord, error) {
	r, err := g.readLaunch(key)
	if err != nil {
		return LaunchRecord{}, err
	}
	if r.State != "pending" || r.Lease != lease || lease == "" {
		return LaunchRecord{}, fmt.Errorf("launch lease is no longer current")
	}
	id, foundKey, fingerprint, found, err := finder.FindRequest(ctx, r.Name)
	if err != nil {
		return LaunchRecord{}, err
	}
	if !found || id != workerID || foundKey != key || fingerprint != r.Fingerprint {
		return LaunchRecord{}, fmt.Errorf("launch result does not match managed worker")
	}
	r.State, r.WorkerID, r.Lease, r.UpdatedAt = "complete", id, "", time.Now().UTC()
	return r, g.writeLaunch(r)
}

func (g *Guard) abortLaunch(key, lease, workerID string) (LaunchRecord, error) {
	r, err := g.readLaunch(key)
	if err != nil {
		return LaunchRecord{}, err
	}
	if r.State != "pending" || r.Lease != lease || lease == "" {
		return LaunchRecord{}, fmt.Errorf("launch lease is no longer current")
	}
	if workerID == "" {
		r.State = "failed"
	} else {
		r.State, r.WorkerID = "cleanup-pending", workerID
	}
	r.Lease, r.UpdatedAt = "", time.Now().UTC()
	return r, g.writeLaunch(r)
}
