//go:build linux

package engine

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/containerd/containerd/v2/core/leases"
	"github.com/containerd/containerd/v2/core/mount"
	"github.com/containerd/continuity/fs"
	"github.com/containerd/errdefs"
	"github.com/opencontainers/image-spec/identity"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	dockerdefaults "github.com/pacificbelt30/codex-dock/docker"
	"github.com/pacificbelt30/codex-dock/internal/diagnostics"
)

func (b *Backend) checkImage(ctx context.Context, name, agent string) (report diagnostics.Report, resultErr error) {
	if agent != "shell" && agent != "codex" && agent != "claude" {
		return report, fmt.Errorf("choose agent shell, codex or claude")
	}
	// A temporary view is not referenced by a container. Keep it leased until
	// the inspection and unmount finish, even when containerd GC runs meanwhile.
	ctx, release, err := b.client.WithLease(ctx, leases.WithRandomID(), leases.WithExpiration(2*time.Minute))
	if err != nil {
		return report, err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		resultErr = errors.Join(resultErr, release(cleanup))
	}()
	img, ic, err := b.image(ctx, name)
	if err != nil {
		return report, err
	}
	if err = img.Unpack(ctx, b.opts.Snapshotter); err != nil {
		return report, err
	}
	layers, err := img.RootFS(ctx)
	if err != nil {
		return report, err
	}
	key, err := id()
	if err != nil {
		return report, err
	}
	key = "image-check-" + key
	service := b.client.SnapshotService(b.opts.Snapshotter)
	mounts, err := service.View(ctx, key, identity.ChainID(layers).String())
	if err != nil {
		return report, err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if err := service.Remove(cleanup, key); err != nil && !errdefs.IsNotFound(err) {
			resultErr = errors.Join(resultErr, err)
		}
	}()
	err = mount.WithReadonlyTempMount(ctx, mounts, func(root string) error {
		report = inspectImageRoot(root, ic, agent)
		descriptor, err := img.Config(ctx)
		if err != nil {
			return err
		}
		report.Add("identity", descriptor.Digest.String(), "", nil)
		return nil
	})
	return report, err
}

func inspectImageRoot(root string, ic v1.Image, agent string) diagnostics.Report {
	var r diagnostics.Report
	var platformErr error
	if ic.OS != "linux" || ic.Architecture != "amd64" {
		platformErr = fmt.Errorf("expected linux/amd64; image is %s/%s", ic.OS, ic.Architecture)
	}
	r.Add("platform", "linux/amd64", "Build a Linux amd64 image for the pinned Dragonball profile.", platformErr)
	paths := "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
	for _, e := range ic.Config.Env {
		if strings.HasPrefix(e, "PATH=") {
			paths = strings.TrimPrefix(e, "PATH=")
		}
	}
	find := func(name string) (string, error) {
		candidates := []string{name}
		if !strings.Contains(name, "/") {
			candidates = nil
			for _, dir := range strings.Split(paths, ":") {
				if filepath.IsAbs(dir) {
					candidates = append(candidates, filepath.Join(dir, name))
				}
			}
		}
		for _, candidate := range candidates {
			if !filepath.IsAbs(candidate) {
				continue
			}
			path, err := fs.RootPath(root, candidate)
			if err != nil {
				continue
			}
			info, err := os.Stat(path)
			if err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0111 != 0 {
				return path, nil
			}
		}
		return "", fmt.Errorf("missing executable: %s", name)
	}
	tools := []string{"bash", "curl", "python3", "git"}
	if agent != "shell" {
		tools = append(tools, agent)
	}
	for _, tool := range tools {
		_, err := find(tool)
		r.Add(tool, "executable found", "Install "+tool+" in the image and make it executable on PATH.", err)
	}
	command := append(append([]string{}, ic.Config.Entrypoint...), ic.Config.Cmd...)
	var commandErr error
	if len(command) == 0 {
		commandErr = fmt.Errorf("image has no ENTRYPOINT or CMD")
	} else {
		_, commandErr = find(command[0])
	}
	r.Add("entrypoint", "image command exists", "Set ENTRYPOINT [\"/entrypoint.sh\"] and COPY the script printed by codex-dock build --print-entrypoint.", commandErr)
	path, err := fs.RootPath(root, "/entrypoint.sh")
	if err == nil {
		var info os.FileInfo
		info, err = os.Stat(path)
		if err == nil && (!info.Mode().IsRegular() || info.Size() > 1<<20) {
			err = fmt.Errorf("entrypoint is not a small regular file")
		}
	}
	var data []byte
	if err == nil {
		data, err = os.ReadFile(path)
	}
	if err != nil || sha256.Sum256(data) != sha256.Sum256(dockerdefaults.Entrypoint) {
		r.Checks = append(r.Checks, diagnostics.Check{Name: "auth-entrypoint", Status: "warning", Detail: "entrypoint differs from this CLI; custom wrappers need a real Kata smoke test", Fix: "Vendor codex-dock build --print-entrypoint or inherit the matching codex-dock base image."})
	} else {
		r.Add("auth-entrypoint", "canonical auth entrypoint matches this CLI", "", nil)
	}
	r.Checks = append(r.Checks, diagnostics.Check{Name: "scope", Status: "ok", Detail: "static read-only checks only; runtime UID, interpreter/libraries and auth/network behavior still require a Kata smoke test"})
	return r
}
