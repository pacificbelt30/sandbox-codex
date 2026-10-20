package sandbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/pacificbelt30/codex-dock/internal/network"
)

type cleanupRecord struct {
	Version            int
	Endpoint, ID, Name string
	Labels             map[string]string
	Mounts             []mount.Mount
	Placeholder        string
	Force              bool
}

var fullContainerID = regexp.MustCompile(`^[a-f0-9]{64}$`)

func (m *Manager) stateDirectory() (string, error) {
	root := m.stateDir
	if value, ok := os.LookupEnv("CODEX_DOCK_STATE_DIR"); ok {
		root = value
	}
	if root == "" {
		root = os.Getenv("XDG_STATE_HOME")
		if root == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			root = filepath.Join(home, ".codex-dock-state")
		}
		sum := sha256.Sum256([]byte(m.cli.DaemonHost()))
		root = filepath.Join(root, "codex-dock", hex.EncodeToString(sum[:16]))
	}
	if !filepath.IsAbs(root) || filepath.Clean(root) != root || root == "/" {
		return "", fmt.Errorf("cleanup state must be a clean absolute private directory")
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return "", err
	}
	for current := root; current != "/"; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil {
			return "", err
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || stat.Uid != 0 && stat.Uid != uint32(os.Geteuid()) || info.Mode().Perm()&0022 != 0 && (current == root || info.Mode()&os.ModeSticky == 0) {
			return "", fmt.Errorf("untrusted cleanup state directory: %s", current)
		}
	}
	return root, nil
}

// Check before reserving runtime resources: rollback also needs this directory.
// Probe actual writes and durability, not just ownership and permission bits.
func (m *Manager) checkCleanupState() error {
	dir, err := m.stateDirectory()
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".write-check-*")
	if err != nil {
		return err
	}
	err = errors.Join(f.Sync(), f.Close(), os.Remove(f.Name()))
	if err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	return errors.Join(d.Sync(), d.Close())
}

func (m *Manager) lockCleanup(ctx context.Context, id string) (func(), error) {
	if !fullContainerID.MatchString(id) {
		return nil, fmt.Errorf("cleanup requires a full container ID")
	}
	dir, err := m.stateDirectory()
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, id+".lock"), os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	for {
		if err = ctx.Err(); err != nil {
			_ = f.Close()
			return nil, err
		}
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
		}
		if err != syscall.EWOULDBLOCK && err != syscall.EAGAIN {
			_ = f.Close()
			return nil, err
		}
		select {
		case <-ctx.Done():
			_ = f.Close()
			return nil, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func recordInspection(r cleanupRecord) container.InspectResponse {
	return container.InspectResponse{ContainerJSONBase: &container.ContainerJSONBase{ID: r.ID, Name: r.Name, HostConfig: &container.HostConfig{Mounts: r.Mounts}}, Config: &container.Config{Labels: r.Labels, Env: []string{"CODEX_TOKEN=" + r.Placeholder}}}
}

func (m *Manager) writeCleanup(r cleanupRecord) error {
	dir, err := m.stateDirectory()
	if err != nil {
		return err
	}
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".cleanup-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer func() { _ = f.Close(); _ = os.Remove(name) }()
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(name, filepath.Join(dir, r.ID+".json")); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer func() { _ = d.Close() }()
	return d.Sync()
}

func (m *Manager) readCleanup(id string) (cleanupRecord, error) {
	var r cleanupRecord
	if !fullContainerID.MatchString(id) {
		return r, fmt.Errorf("invalid cleanup identity")
	}
	dir, err := m.stateDirectory()
	if err != nil {
		return r, err
	}
	path := filepath.Join(dir, id+".json")
	info, err := os.Lstat(path)
	if err != nil {
		return r, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return r, fmt.Errorf("untrusted cleanup record")
	}
	f, err := os.Open(path)
	if err != nil {
		return r, err
	}
	defer func() { _ = f.Close() }()
	decoder := json.NewDecoder(io.LimitReader(f, 1024*1024))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&r); err != nil {
		return r, err
	}
	var extra any
	if err = decoder.Decode(&extra); err != io.EOF {
		return r, fmt.Errorf("trailing cleanup record data")
	}
	if r.Version != 1 || r.ID != id || r.Endpoint != m.cli.DaemonHost() || r.Labels[labelManaged] != "true" || !strings.HasPrefix(r.Name, "/") {
		return r, fmt.Errorf("cleanup ownership/daemon mismatch")
	}
	return r, nil
}

func (m *Manager) pendingCleanup() ([]cleanupRecord, error) {
	dir, err := m.stateDirectory()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var records []cleanupRecord
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		r, err := m.readCleanup(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, nil
}

func (m *Manager) findCleanup(value string) (cleanupRecord, error) {
	records, err := m.pendingCleanup()
	if err != nil {
		return cleanupRecord{}, err
	}
	var found *cleanupRecord
	for i := range records {
		r := &records[i]
		if r.ID == value || strings.TrimPrefix(r.Name, "/") == value || len(value) >= 12 && strings.HasPrefix(r.ID, value) {
			if found != nil {
				return cleanupRecord{}, fmt.Errorf("ambiguous pending cleanup")
			}
			found = r
		}
	}
	if found == nil {
		return cleanupRecord{}, fmt.Errorf("worker or pending cleanup %q: %w", value, errdefs.ErrNotFound)
	}
	return *found, nil
}

// RetryCleanup recovers interrupted removals, including after a CLI restart.
func (m *Manager) RetryCleanup(force bool) error {
	records, err := m.pendingCleanup()
	if err != nil {
		return err
	}
	var failures error
	for _, r := range records {
		if err := m.Remove(r.ID, force || r.Force); err != nil {
			failures = errors.Join(failures, fmt.Errorf("retry cleanup %s: %w", r.Name, err))
		}
	}
	return failures
}

func (m *Manager) removeDurably(parent context.Context, value string, force bool) error {
	ctx, cancel := context.WithTimeout(parent, 90*time.Second)
	defer cancel()
	v, err := m.cli.ContainerInspect(ctx, value)
	var record cleanupRecord
	if err != nil {
		if !errdefs.IsNotFound(err) {
			return err
		}
		record, err = m.findCleanup(value)
		if err != nil {
			return err
		}
	} else {
		if v.ContainerJSONBase == nil || v.Config == nil || v.HostConfig == nil || v.Config.Labels[labelManaged] != "true" {
			return fmt.Errorf("refusing to remove an unmanaged/incomplete worker")
		}
		labels := map[string]string{}
		for _, key := range []string{labelManaged, labelIsolation, labelRuntime, labelProxyHosts, labelAllocation, labelProfile, labelPrefix + "network-id", labelPrefix + "worktree", labelPrefix + "proxy-admin-url", labelPrefix + "proxy-container-url", labelPrefix + "token-ttl"} {
			if value, ok := v.Config.Labels[key]; ok {
				labels[key] = value
			}
		}
		record = cleanupRecord{Version: 1, Endpoint: m.cli.DaemonHost(), ID: v.ID, Name: v.Name, Labels: labels, Mounts: v.HostConfig.Mounts, Placeholder: workerPlaceholder(v), Force: force}
	}
	unlock, err := m.lockCleanup(ctx, record.ID)
	if err != nil {
		return err
	}
	defer unlock()
	if saved, err := m.readCleanup(record.ID); err == nil {
		record = saved
		record.Force = record.Force || force
	} else if !os.IsNotExist(err) {
		return err
	}
	// The recoverable record is durable BEFORE deleting the only Docker object
	// carrying its network/worktree identity. No raw user task/env is stored.
	if err = m.writeCleanup(record); err != nil {
		return fmt.Errorf("persist cleanup intent: %w", err)
	}
	if err = m.cli.ContainerRemove(ctx, record.ID, container.RemoveOptions{Force: record.Force}); err != nil && !errdefs.IsNotFound(err) {
		return err
	}
	v = recordInspection(record)
	revokeErr := m.revokeWorkerTokenChecked(v)
	if record.Labels[labelIsolation] == RuntimeKata {
		var proxies []string
		if err = json.Unmarshal([]byte(record.Labels[labelProxyHosts]), &proxies); err != nil {
			return err
		}
		netMgr := m.network
		if netMgr == nil {
			netMgr = network.NewManagerWithClient(m.cli)
		}
		if err = netMgr.RemoveKataWorkerNetwork(ctx, record.Labels[labelPrefix+"network-id"], proxies); err != nil {
			return fmt.Errorf("worker removed; retry rm to complete network cleanup: %w", err)
		}
		if record.Labels[labelAllocation] != "" {
			if err = m.releaseKataAllocation(ctx, record.Labels[labelAllocation], record.ID); err != nil {
				return err
			}
		}
	} else if m.network != nil {
		if err = m.network.RemoveWorkerNetwork(strings.TrimPrefix(record.Name, "/")); err != nil {
			return err
		}
	}
	if err = cleanupManagedWorktree(ctx, v); err != nil {
		return err
	}
	if revokeErr != nil {
		return fmt.Errorf("worker removed; retry rm to revoke its token: %w", revokeErr)
	}
	dir, err := m.stateDirectory()
	if err != nil {
		return err
	}
	if err = os.Remove(filepath.Join(dir, record.ID+".json")); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer func() { _ = d.Close() }()
	return d.Sync()
}
