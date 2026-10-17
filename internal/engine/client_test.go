package engine

import (
	"strings"
	"testing"
)

func TestNodeClientIgnoresDockerEndpoint(t *testing.T) {
	t.Setenv("DOCKER_HOST", "tcp://untrusted.invalid:2375")
	t.Setenv("CODEX_DOCK_KATA_NODE_SOCKET", "/run/verification-node.sock")
	c, err := NewClient()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	if c.DaemonHost() != "unix:///run/verification-node.sock" {
		t.Fatal("ambient Docker endpoint affected native node")
	}
}
func TestBuildTagsRejectExporterOptionInjection(t *testing.T) {
	for _, tag := range []string{"image,compression=uncompressed", "image\nother", "--output"} {
		if _, err := normalizeBuildName(tag); err == nil {
			t.Fatalf("accepted invalid tag %q", tag)
		}
	}
	tag, err := normalizeBuildName("custom-worker:dev")
	if err != nil || !strings.HasSuffix(tag, "/custom-worker:dev") {
		t.Fatalf("normalization: %s %v", tag, err)
	}
}
