// Package storetest provides a reusable conformance suite for host-side Store
// adapters. Run it against isolated test storage, never production credentials.
package storetest

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"

	"github.com/pacificbelt30/codex-dock/pkg/cxd/credentialstore"
)

// Harness supplies adapter-specific fixture controls. Replace models a new
// login, Delete disconnects a case, and FailNextSave injects a known pre-commit
// durable-write failure (not an ambiguous lost acknowledgement). Reopen must create a fresh adapter over the same durable database.
// Each factory invocation must own independent storage and register cleanup.
type Harness struct {
	Store        credentialstore.Store
	Replace      func(context.Context, string, credentialstore.Credentials) error
	Delete       func(context.Context, string) error
	Reopen       func(context.Context) (credentialstore.Store, error)
	FailNextSave func(context.Context) error
}

// Run checks version/connection separation, strict atomic CAS across adapter
// instances, persistence, failure atomicity, revocation and snapshot ownership.
// Assertions deliberately never print credential contents.
func Run(t *testing.T, factory func(*testing.T) Harness) {
	t.Helper()
	ctx := context.Background()
	fresh := func(t *testing.T) Harness {
		t.Helper()
		h := factory(t)
		if h.Store == nil || h.Replace == nil || h.Delete == nil || h.Reopen == nil || h.FailNextSave == nil {
			t.Fatal("all Store harness controls are required")
		}
		for _, id := range []string{"case-a", "case-b"} {
			if err := h.Replace(ctx, id, initial(id)); err != nil {
				t.Fatal("could not seed isolated credential fixture")
			}
		}
		return h
	}
	load := func(t *testing.T, s credentialstore.Store, id string) credentialstore.Credentials {
		t.Helper()
		v, err := s.Load(ctx, id)
		if err != nil {
			t.Fatal("loading seeded credentials failed")
		}
		return v
	}
	assertSaved := func(t *testing.T, h Harness, want credentialstore.Credentials) {
		t.Helper()
		for _, s := range []credentialstore.Store{h.Store, reopen(t, ctx, h)} {
			if !reflect.DeepEqual(load(t, s, "case-a"), want) {
				t.Fatal("acknowledged state was not durably preserved")
			}
		}
	}
	t.Run("missing", func(t *testing.T) {
		h := fresh(t)
		if _, err := h.Store.Load(ctx, "missing"); !errors.Is(err, credentialstore.ErrNotFound) {
			t.Fatal("missing load must return ErrNotFound")
		}
		if _, err := h.Store.SaveRefresh(ctx, "missing", "v1", "openai", initial("missing")); !errors.Is(err, credentialstore.ErrNotFound) {
			t.Fatal("refresh must not create a missing connection")
		}
	})
	for _, provider := range []string{"openai", "anthropic"} {
		t.Run("refresh-"+provider, func(t *testing.T) {
			h := fresh(t)
			before := load(t, h.Store, "case-a")
			other := load(t, h.Store, "case-b")
			next := initial("case-a")
			rotate(&next, provider, "rotated")
			version, err := h.Store.SaveRefresh(ctx, "case-a", before.Version, provider, next)
			if err != nil || version == "" || version == before.Version {
				t.Fatal("refresh must commit and return a new credential version")
			}
			next.Version = version
			assertSaved(t, h, next)
			if !reflect.DeepEqual(load(t, h.Store, "case-b"), other) {
				t.Fatal("refresh changed another case")
			}
			if _, err := h.Store.SaveRefresh(ctx, "case-a", before.Version, provider, initial("case-a")); !errors.Is(err, credentialstore.ErrConflict) {
				t.Fatal("stale refresh must return ErrConflict, never success")
			}
			assertSaved(t, h, next)
		})
	}
	t.Run("concurrent-cas-across-instances", func(t *testing.T) {
		h := fresh(t)
		second := reopen(t, ctx, h)
		before := load(t, h.Store, "case-a")
		type result struct {
			version string
			next    credentialstore.Credentials
			err     error
		}
		const count = 12
		results := make(chan result, count)
		start := make(chan struct{})
		var ready sync.WaitGroup
		ready.Add(count)
		for i := 0; i < count; i++ {
			go func() {
				next := initial("case-a")
				rotate(&next, "openai", fmt.Sprintf("winner-%d", i))
				s := h.Store
				if i%2 == 1 {
					s = second
				}
				ready.Done()
				<-start
				version, err := s.SaveRefresh(ctx, "case-a", before.Version, "openai", next)
				results <- result{version, next, err}
			}()
		}
		ready.Wait()
		close(start)
		wins := 0
		var winner credentialstore.Credentials
		for i := 0; i < count; i++ {
			r := <-results
			if r.err == nil {
				wins++
				winner = r.next
				winner.Version = r.version
				if r.version == "" || r.version == before.Version {
					t.Error("CAS winner returned an invalid version")
				}
			} else if !errors.Is(r.err, credentialstore.ErrConflict) {
				t.Error("CAS loser must report ErrConflict")
			}
		}
		if wins != 1 {
			t.Fatalf("CAS must have exactly one winner, got %d", wins)
		}
		assertSaved(t, h, winner)
	})
	t.Run("connection-replacement-and-disconnect", func(t *testing.T) {
		h := fresh(t)
		old := load(t, h.Store, "case-a")
		replacement := initial("case-a")
		replacement.Version = "login-v2"
		replacement.ConnectionVersion = "connection-v2"
		if err := h.Replace(ctx, "case-a", replacement); err != nil {
			t.Fatal("fixture login replacement failed")
		}
		if _, err := h.Store.SaveRefresh(ctx, "case-a", old.Version, "openai", old); !errors.Is(err, credentialstore.ErrConflict) {
			t.Fatal("old login overwrote replacement")
		}
		assertSaved(t, h, replacement)
		if err := h.Delete(ctx, "case-a"); err != nil {
			t.Fatal("fixture disconnect failed")
		}
		if _, err := h.Store.SaveRefresh(ctx, "case-a", replacement.Version, "openai", replacement); !errors.Is(err, credentialstore.ErrNotFound) {
			t.Fatal("refresh resurrected a disconnected case")
		}
		if _, err := reopen(t, ctx, h).Load(ctx, "case-a"); !errors.Is(err, credentialstore.ErrNotFound) {
			t.Fatal("disconnect was not durable")
		}
		replacement.Version = "login-v3"
		replacement.ConnectionVersion = "connection-v3"
		if err := h.Replace(ctx, "case-a", replacement); err != nil {
			t.Fatal("fixture reconnect failed")
		}
		if _, err := h.Store.SaveRefresh(ctx, "case-a", old.Version, "openai", old); !errors.Is(err, credentialstore.ErrConflict) {
			t.Fatal("stale refresh crossed reconnect")
		}
		assertSaved(t, h, replacement)
	})
	t.Run("write-failure-is-atomic", func(t *testing.T) {
		h := fresh(t)
		before := load(t, h.Store, "case-a")
		next := initial("case-a")
		rotate(&next, "openai", "not-committed")
		if err := h.FailNextSave(ctx); err != nil {
			t.Fatal("could not arm durable-write failure")
		}
		if _, err := h.Store.SaveRefresh(ctx, "case-a", before.Version, "openai", next); err == nil {
			t.Fatal("failed persistence was acknowledged")
		}
		assertSaved(t, h, before)
	})
	t.Run("canceled-operation-does-not-write", func(t *testing.T) {
		h := fresh(t)
		before := load(t, h.Store, "case-a")
		canceled, cancel := context.WithCancel(ctx)
		cancel()
		if _, err := h.Store.Load(canceled, "case-a"); !errors.Is(err, context.Canceled) {
			t.Fatal("Load did not preserve cancellation")
		}
		next := initial("case-a")
		rotate(&next, "openai", "canceled")
		if _, err := h.Store.SaveRefresh(canceled, "case-a", before.Version, "openai", next); !errors.Is(err, context.Canceled) {
			t.Fatal("SaveRefresh did not preserve cancellation")
		}
		assertSaved(t, h, before)
	})
	t.Run("credentials-are-owned-snapshots", func(t *testing.T) {
		h := fresh(t)
		before := initial("case-a")
		v := load(t, h.Store, "case-a")
		v.OpenAIOAuth.AccessToken = "mutated"
		v.AnthropicOAuth.Scopes[0] = "mutated"
		assertSaved(t, h, before)
		next := initial("case-a")
		rotate(&next, "openai", "saved")
		version, err := h.Store.SaveRefresh(ctx, "case-a", before.Version, "openai", next)
		if err != nil {
			t.Fatal("snapshot save failed")
		}
		want := initial("case-a")
		rotate(&want, "openai", "saved")
		want.Version = version
		next.OpenAIOAuth.AccessToken = "mutated-after-save"
		next.AnthropicOAuth.Scopes[0] = "mutated-after-save"
		assertSaved(t, h, want)
	})
}

func reopen(t *testing.T, ctx context.Context, h Harness) credentialstore.Store {
	t.Helper()
	s, err := h.Reopen(ctx)
	if err != nil || s == nil {
		t.Fatal("reopening durable adapter failed")
	}
	return s
}
func initial(id string) credentialstore.Credentials {
	return credentialstore.Credentials{Version: "credential-v1", ConnectionVersion: "connection-v1", OpenAIOAuth: &credentialstore.OpenAIOAuth{AccessToken: id + "-access", RefreshToken: id + "-refresh"}, AnthropicOAuth: &credentialstore.AnthropicOAuth{AccessToken: id + "-anthropic", RefreshToken: id + "-anthropic-refresh", Scopes: []string{"fixture-scope"}}}
}
func rotate(v *credentialstore.Credentials, provider, value string) {
	if provider == "openai" {
		v.OpenAIOAuth.AccessToken = value
		v.OpenAIOAuth.RefreshToken = value + "-refresh"
	} else {
		v.AnthropicOAuth.AccessToken = value
		v.AnthropicOAuth.RefreshToken = value + "-refresh"
	}
}
