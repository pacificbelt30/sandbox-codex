//go:build linux

package engine

import (
	"context"
	"fmt"
	"testing"

	cd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/containers"
	"github.com/containerd/containerd/v2/pkg/namespaces"
)

type requestNamespaceStore struct {
	containers.Store
	calls int
}

func (s *requestNamespaceStore) List(ctx context.Context, _ ...string) ([]containers.Container, error) {
	ns, err := namespaces.NamespaceRequired(ctx)
	if err != nil {
		return nil, err
	}
	if ns != Namespace {
		return nil, fmt.Errorf("request lookup escaped product namespace: %s", ns)
	}
	s.calls++
	return nil, nil
}

func TestFindRequestPinsContainerdNamespace(t *testing.T) {
	for _, ns := range []string{"", "another-product"} {
		t.Run("incoming-"+ns, func(t *testing.T) {
			store := &requestNamespaceStore{}
			client, err := cd.New("", cd.WithServices(cd.WithContainerStore(store)))
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			ctx := context.Background()
			if ns != "" {
				ctx = namespaces.WithNamespace(ctx, ns)
			}
			backend := &Backend{client: client}
			_, _, _, found, err := backend.FindRequest(ctx, "dock-request-not-yet-created")
			if err != nil || found || store.calls != 1 {
				t.Fatalf("lookup without a worker: found=%v calls=%d err=%v", found, store.calls, err)
			}
		})
	}
}
