package authproxy

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pacificbelt30/codex-dock/pkg/cxd/credentialstore"
	"github.com/pacificbelt30/codex-dock/pkg/cxd/identity"
)

type caseStore struct {
	items map[string]credentialstore.Credentials
	fail  bool
}

func (s *caseStore) Load(_ context.Context, id string) (credentialstore.Credentials, error) {
	v, ok := s.items[id]
	if !ok {
		return credentialstore.Credentials{}, credentialstore.ErrNotFound
	}
	return v, nil
}

func (s *caseStore) SaveRefresh(_ context.Context, id, version, provider string, next credentialstore.Credentials) (string, error) {
	if s.fail {
		return "", errors.New("database unavailable")
	}
	current, ok := s.items[id]
	if !ok {
		return "", credentialstore.ErrNotFound
	}
	if current.Version != version {
		return "", credentialstore.ErrConflict
	}
	if provider != "openai" {
		return "", errors.New("unexpected provider")
	}
	next.Version = version + "-next"
	s.items[id] = next
	return next.Version, nil
}

func TestExternalCredentialCasesDoNotReadHostOrMixRefreshes(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "host-secret-must-not-be-used")
	store := &caseStore{items: map[string]credentialstore.Credentials{
		"user-a": {Version: "a1", ConnectionVersion: "v1", OpenAIOAuth: &credentialstore.OpenAIOAuth{AccessToken: "access-a", RefreshToken: "refresh-a"}},
		"user-b": {Version: "b1", ConnectionVersion: "v1", OpenAIOAuth: &credentialstore.OpenAIOAuth{AccessToken: "access-b", RefreshToken: "refresh-b"}},
	}}
	a, err := NewProxy(Config{CredentialStore: store, CaseID: "user-a", AdminSecret: "fixture-admin-secret-long-enough-0001", AdminListenAddr: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewProxy(Config{CredentialStore: store, CaseID: "user-b", AdminSecret: "fixture-admin-secret-long-enough-0002", AdminListenAddr: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	if a.apiKey != "" || b.apiKey != "" || a.oauthCreds.AccessToken != "access-a" || b.oauthCreds.AccessToken != "access-b" {
		t.Fatal("credential cases mixed or host credentials loaded")
	}
	tokenA, err := a.IssueTokenWithIdentity("worker-a", 60, identity.Identity{CaseID: "user-a", RunID: "run-a", ConnectionVersion: "v1", Provider: "openai"})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	request.Header.Set("Authorization", "Bearer "+tokenA)
	if _, cleanup, ok := b.workerRequest(request, true); ok {
		cleanup()
		t.Fatal("A token was accepted by B proxy")
	}
	next := *a.oauthCreds
	next.AccessToken = "new-access-a"
	active, release, ok := a.workerRequest(request, true)
	if !ok {
		t.Fatal("case token rejected before storage failure")
	}
	defer release()
	store.fail = true
	if err := a.persistOAuthState("openai", &next); err == nil {
		t.Fatal("failed DB save was accepted")
	}
	if a.storeVersion != "a1" || a.oauthCreds.AccessToken != "access-a" {
		t.Fatal("failed save published new credentials")
	}
	select {
	case <-active.Context().Done():
	case <-time.After(time.Second):
		t.Fatal("storage failure left active stream lease alive")
	}
	if _, cleanup, ok := a.workerRequest(request, true); ok {
		cleanup()
		t.Fatal("storage failure did not revoke the case token")
	}
	store.fail = false
	// Recovery requires a fresh proxy that reloads the durable generation.
	a, err = NewProxy(Config{CredentialStore: store, CaseID: "user-a", AdminSecret: "fixture-admin-secret-long-enough-0001", AdminListenAddr: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.persistOAuthState("openai", &next); err != nil {
		t.Fatal(err)
	}
	if store.items["user-b"].OpenAIOAuth.AccessToken != "access-b" {
		t.Fatal("B credentials changed")
	}
	if store.items["user-a"].OpenAIOAuth.AccessToken != "new-access-a" {
		t.Fatal("A refresh not saved")
	}
	restarted, err := NewProxy(Config{CredentialStore: store, CaseID: "user-a", AdminSecret: "fixture-admin-secret-long-enough-0001", AdminListenAddr: "127.0.0.1:0"})
	if err != nil || restarted.oauthCreds.AccessToken != "new-access-a" {
		t.Fatalf("updated credential was not restored after proxy restart: %v", err)
	}
	// A re-login or connection replacement changes the store version. A stale
	// proxy must fail the CAS instead of publishing a late refresh.
	replacement := store.items["user-a"]
	replacement.Version = "new-login"
	replacement.ConnectionVersion = "v2"
	store.items["user-a"] = replacement
	if err := a.persistOAuthState("openai", &next); !errors.Is(err, credentialstore.ErrConflict) {
		t.Fatalf("stale refresh after new login was accepted: %v", err)
	}
	if _, err := NewProxy(Config{CredentialStore: store, CaseID: "user-a", OAuthStateDir: t.TempDir(), AdminSecret: "fixture-admin-secret-long-enough-0001", AdminListenAddr: "127.0.0.1:0"}); err == nil {
		t.Fatal("external store mixed with local OAuth state")
	}
}
