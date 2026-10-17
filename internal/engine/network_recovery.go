//go:build linux

package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	cd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/errdefs"
)

// After a host reboot, container records/snapshots survive but kernel bridges,
// namespaces and firewall tables do not. Rebuild only absent owned resources,
// and only when neither containerd nor the kernel reports a live occupant.
// Existing resources must match their saved ownership; drift is never adopted.
func (b *Backend) recoverNetworks(ctx context.Context, containers []cd.Container) error {
	records := map[string]*record{}
	live := map[string]bool{}
	for _, c := range containers {
		r, _, err := b.load(ctx, c.ID())
		if err != nil {
			return err
		}
		records[r.ID] = r
		_, err = c.Task(ctx, nil)
		if err == nil {
			live[r.ID] = true
		} else if !errdefs.IsNotFound(err) {
			return err
		}
	}
	networks, err := b.networks()
	if err != nil {
		return err
	}
	for _, n := range networks {
		_, err := os.Stat("/sys/class/net/" + n.Bridge)
		if os.IsNotExist(err) {
			for cid := range n.Containers {
				r := records[cid]
				if r == nil {
					return fmt.Errorf("network has an unreconciled creation/removal intent: %s", cid)
				}
				if live[cid] {
					return fmt.Errorf("live task lost its bridge")
				}
				if err = b.kernelIdle(r); err != nil {
					return err
				}
			}
			// A surviving table is removable only if its exact saved rules match.
			if hash, e := b.firewallHash(ctx, n); e == nil {
				if n.Firewall == "" || hash != n.Firewall {
					return fmt.Errorf("stale firewall ownership changed")
				}
				if err = b.nft(ctx, "delete table inet "+table(n)+"\n"); err != nil {
					return err
				}
			}
			if err = b.materializeNetwork(ctx, n); err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else if n.Firewall != "" {
			if err = b.verifyNetwork(ctx, n); err != nil {
				return err
			}
		}
	}
	for _, r := range records {
		info, err := os.Stat(b.nsPath(r.ID))
		if err == nil {
			s, ok := info.Sys().(*syscall.Stat_t)
			if !ok || uint64(s.Dev) != r.NetNSDevice || s.Ino != r.NetNSInode {
				return fmt.Errorf("namespace identity drift")
			}
			continue
		}
		if !os.IsNotExist(err) {
			return err
		}
		if live[r.ID] {
			return fmt.Errorf("live task lost its network namespace")
		}
		if err = b.kernelIdle(r); err != nil {
			return err
		}
		for name, e := range r.Networks {
			n, err := b.network(name)
			if err != nil {
				return err
			}
			if err = b.deleteEndpointLink(ctx, n, r.ID, e.EndpointID); err != nil {
				return err
			}
		}
		if err = b.newNamespace(ctx, r); err != nil {
			return err
		}
		names := make([]string, 0, len(r.Networks))
		for name := range r.Networks {
			names = append(names, name)
		}
		sort.Slice(names, func(i, j int) bool {
			return r.Networks[names[i]].DriverOpts["codex-dock.interface"] < r.Networks[names[j]].DriverOpts["codex-dock.interface"]
		})
		for _, name := range names {
			n, err := b.network(name)
			if err != nil {
				return err
			}
			e := r.Networks[name]
			if n.Containers[r.ID].EndpointID != e.EndpointID {
				return fmt.Errorf("saved endpoint identity drift")
			}
			if err = b.materializeEndpoint(ctx, n, r, e); err != nil {
				return err
			}
		}
	}
	return nil
}
func (b *Backend) kernelIdle(r *record) error {
	path := r.Host.CgroupParent
	if path == "" {
		if r.Spec.Linux != nil {
			path = r.Spec.Linux.CgroupsPath
		} else {
			path = "/codex-dock-services/" + r.ID
		}
	}
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || path == "/" {
		return fmt.Errorf("invalid task cgroup")
	}
	data, err := os.ReadFile(filepath.Join("/sys/fs/cgroup", path, "cgroup.events"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !strings.Contains(string(data), "populated 0") {
		return fmt.Errorf("kernel still reports a live task")
	}
	return nil
}
