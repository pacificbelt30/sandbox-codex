package authproxy

import (
	"context"
	"errors"
	"testing"
)

func TestRemoteProxyInitialProbeHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewRemoteProxyContext(ctx, "http://127.0.0.1:1", "http://proxy:18080", "")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled probe returned %v", err)
	}
}
