package cxd

import (
	"context"
	"strings"
	"testing"
)

func TestLaunchWithTimingsPreservesValidationFailure(t *testing.T) {
	ctx := context.Background()
	c := &Client{}
	w, timing, err := c.LaunchWithTimings(ctx, LaunchRequest{})
	_, ordinary := c.Launch(ctx, LaunchRequest{})
	if err == nil || ordinary == nil || err.Error() != ordinary.Error() || w.ID != "" {
		t.Fatalf("invalid call changed its outcome: worker=%+v error=%v ordinary=%v", w, err, ordinary)
	}
	if len(timing.Phases) != 1 || timing.Phases[0].Name != "request_validation" || timing.Phases[0].Duration != timing.Duration || timing.Duration < 0 {
		t.Fatalf("missing partial timing for validation failure: %+v", timing)
	}
}

func TestOpenRejectsInvalidHostEgressBeforeConnecting(t *testing.T) {
	_, err := Open(context.Background(), Options{EgressPolicy: EgressPolicy{AllowDomains: []string{"=*.example.com"}}})
	if err == nil || !strings.Contains(err.Error(), "invalid host egress policy") {
		t.Fatalf("invalid maximum policy reached the node: %v", err)
	}
}
