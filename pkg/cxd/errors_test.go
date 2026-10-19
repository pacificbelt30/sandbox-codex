package cxd

import (
	"context"
	"errors"
	"testing"

	"github.com/containerd/errdefs"
	"github.com/pacificbelt30/codex-dock/internal/kataguard"
)

func TestPublicErrorCategoriesPreserveCauses(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   error
		want error
	}{
		{"conflict", &kataguard.NodeError{Code: "request_conflict", Message: "different options"}, ErrConflict},
		{"capacity", &kataguard.NodeError{Code: "capacity_exhausted", Message: "full"}, ErrCapacity},
		{"version", &kataguard.VersionMismatchError{ClientVersion: "a", NodeVersion: "b"}, ErrVersionMismatch},
		{"missing", errdefs.ErrNotFound, ErrNotFound},
		{"canceled", context.Canceled, context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := classify(tc.in)
			if !errors.Is(got, tc.want) || !errors.Is(got, tc.in) {
				t.Fatalf("classification %v lost category/cause: %v", tc.in, got)
			}
		})
	}
}

func TestLaunchRequiresStableRequestIDBeforeNodeWork(t *testing.T) {
	c := &Client{}
	if _, err := c.Launch(context.Background(), LaunchRequest{ProjectDir: "/tmp/project"}); err == nil {
		t.Fatal("launch without a request ID was accepted")
	}
}
