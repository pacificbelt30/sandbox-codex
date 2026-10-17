// Package engine implements the node's containerd execution service. The small
// HTTP compatibility surface keeps lifecycle DTOs independent of the SDK. It
// does not connect to, discover, or start a Docker daemon.
package engine

import (
	"fmt"
	"github.com/distribution/reference"
	"github.com/docker/docker/client"
	"os"
	"path/filepath"
)

const DefaultSocket = "/run/codex-dock-node.sock"
const Namespace = "codex-dock"

func Socket() string {
	if p := os.Getenv("CODEX_DOCK_KATA_NODE_SOCKET"); p != "" {
		return p
	}
	return DefaultSocket
}

func NewClient() (*client.Client, error) {
	p := Socket()
	if !filepath.IsAbs(p) || filepath.Clean(p) != p {
		return nil, fmt.Errorf("node socket must be an absolute clean path")
	}
	return client.NewClientWithOpts(client.WithHost("unix://"+p), client.WithVersion("1.51"))
}

func normalizeBuildName(tag string) (string, error) {
	ref, err := reference.ParseNormalizedNamed(tag)
	if err != nil {
		return "", err
	}
	return reference.TagNameOnly(ref).String(), nil
}
