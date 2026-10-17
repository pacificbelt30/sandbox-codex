//go:build linux

package engine

import (
	"github.com/docker/docker/api/types/container"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGuestResolverFilesAreReadableWithoutExposingPrivateMetadata(t *testing.T) {
	b := &Backend{opts: Options{State: t.TempDir()}}
	r := &record{ID: strings.Repeat("a", 64), Name: "worker", Host: container.HostConfig{DNS: []string{"127.0.0.1"}, ExtraHosts: []string{"auth:10.253.1.2"}}}
	if err := b.save(r); err != nil {
		t.Fatal(err)
	}
	if err := b.resolverFiles(r); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"hosts", "resolv.conf", "hostname"} {
		info, err := os.Stat(filepath.Join(b.path(r.ID), file))
		if err != nil || info.Mode().Perm()&0444 != 0444 {
			t.Fatalf("non-root guest cannot read %s: %v", file, err)
		}
	}
	info, err := os.Stat(filepath.Join(b.path(r.ID), "record.json"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("private container metadata exposed: %v", err)
	}
	dir, err := os.Stat(b.path(r.ID))
	if err != nil || dir.Mode().Perm() != 0700 {
		t.Fatalf("private host directory exposed: %v", err)
	}
}
