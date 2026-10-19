package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"

	"github.com/pacificbelt30/codex-dock/internal/buildinfo"
)

var codexVersionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$`)

// Build retains Dockerfile/custom/template builds using standalone BuildKit.
// OCI output is streamed into the same containerd namespace as workers. Neither
// Docker Engine nor its CLI is involved, and no large archive is left on disk.
func Build(ctx context.Context, tag, dockerfile, buildContext string, log io.Writer) error {
	name, err := normalizeBuildName(tag)
	if err != nil {
		return err
	}
	df, err := filepath.Abs(dockerfile)
	if err != nil {
		return err
	}
	dir, err := filepath.Abs(buildContext)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	addr := os.Getenv("BUILDKIT_HOST")
	if addr == "" {
		addr = "unix:///run/codex-dock-buildkit/buildkitd.sock"
	}
	c := exec.CommandContext(ctx, "buildctl", "--addr", addr, "build", "--progress=plain", "--frontend", "dockerfile.v0", "--local", "context="+dir, "--local", "dockerfile="+filepath.Dir(df), "--opt", "filename="+filepath.Base(df), "--opt", "image-resolve-mode=local", "--output", "type=oci,name="+name+",dest=-")
	c.Args = append(c.Args, "--opt", "build-arg:CODEX_DOCK_BUILD_VERSION="+buildinfo.Version)
	if version := os.Getenv("CODEX_DOCK_CODEX_VERSION"); version != "" {
		if !codexVersionPattern.MatchString(version) {
			return fmt.Errorf("invalid CODEX_DOCK_CODEX_VERSION %q: expected an exact semver", version)
		}
		c.Args = append(c.Args, "--opt", "build-arg:CODEX_CLI_VERSION="+version)
	}
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "no_proxy", "all_proxy"} {
		if value := os.Getenv(key); value != "" {
			c.Args = append(c.Args, "--opt", "build-arg:"+key+"="+value)
		}
	}
	if cert := os.Getenv("CODEX_PROXY_CERT"); cert != "" {
		c.Args = append(c.Args, "--secret", "id=proxy_ca,src="+cert)
	}
	c.Stderr = log
	output, err := c.StdoutPipe()
	if err != nil {
		return err
	}
	client, err := NewClient()
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()
	if err = c.Start(); err != nil {
		return fmt.Errorf("standalone BuildKit (buildctl) is required: %w", err)
	}
	response, importErr := client.ImageLoad(ctx, output)
	if importErr == nil {
		_, importErr = io.Copy(log, response.Body)
		importErr = errors.Join(importErr, response.Body.Close())
	}
	if importErr != nil {
		cancel()
	}
	buildErr := c.Wait()
	if err = errors.Join(buildErr, importErr); err != nil {
		return fmt.Errorf("BuildKit build/import failed: %w", err)
	}
	return nil
}
