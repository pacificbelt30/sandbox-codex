package authproxy

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pacificbelt30/codex-dock/pkg/cxd/credentialstore"
	"github.com/pacificbelt30/codex-dock/pkg/cxd/identity"
)

func TestCaseTokenIdentityRestrictsRoutesAndRevocation(t *testing.T) {
	store := &caseStore{items: map[string]credentialstore.Credentials{"user-a": {Version: "v1", ConnectionVersion: "v1", OpenAIAPIKey: "case-secret"}}}
	p, err := NewProxy(Config{CredentialStore: store, CaseID: "user-a", AdminSecret: "fixture-admin-secret-long-enough-0001", AdminListenAddr: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	id := identity.Identity{CaseID: "user-a", RunID: "case-1", ConnectionVersion: "v1", Provider: "openai"}
	if _, err := p.IssueTokenWithIdentity("worker-b", 60, identity.Identity{CaseID: "user-b", RunID: "case-1", ConnectionVersion: "v1", Provider: "openai"}); err == nil {
		t.Fatal("another case was accepted")
	}
	token, err := p.IssueTokenWithIdentity("worker-a", 60, id)
	if err != nil {
		t.Fatal(err)
	}
	request := func(path string) *http.Request {
		r := httptest.NewRequest(http.MethodPost, path, nil)
		r.Header.Set("Authorization", "Bearer "+token)
		return r
	}
	if _, cleanup, ok := p.workerRequest(request("/anthropic/v1/messages"), true); ok {
		cleanup()
		t.Fatal("OpenAI token reached Anthropic route")
	}
	active, cleanup, ok := p.workerRequest(request("/v1/responses"), true)
	if !ok {
		t.Fatal("allowed route rejected")
	}
	defer cleanup()
	revoke := httptest.NewRequest(http.MethodPost, "/admin/revoke-run", nil)
	revoke.Header.Set("X-Proxy-Admin-Secret", "fixture-admin-secret-long-enough-0001")
	// Admin handler validates the full identity before revoking the lease.
	bad := httptest.NewRecorder()
	p.handleAdminRevokeRun(bad, revoke)
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("empty identity status = %d", bad.Code)
	}
	encoded, err := json.Marshal(id)
	if err != nil {
		t.Fatal(err)
	}
	revoke = httptest.NewRequest(http.MethodPost, "/admin/revoke-run", bytes.NewReader(encoded))
	revoke.Header.Set("X-Proxy-Admin-Secret", "fixture-admin-secret-long-enough-0001")
	good := httptest.NewRecorder()
	p.handleAdminRevokeRun(good, revoke)
	if good.Code != http.StatusNoContent {
		t.Fatalf("revoke run status = %d", good.Code)
	}
	select {
	case <-active.Context().Done():
	case <-time.After(time.Second):
		t.Fatal("active stream survived run revocation")
	}
	if _, err := p.IssueTokenWithIdentity("worker-a", 60, id); err == nil {
		t.Fatal("revoked run received another token")
	}
	revokeCase := httptest.NewRequest(http.MethodPost, "/admin/revoke-case", nil)
	revokeCase.Header.Set("X-Proxy-Admin-Secret", "fixture-admin-secret-long-enough-0001")
	caseResult := httptest.NewRecorder()
	p.handleAdminRevokeCase(caseResult, revokeCase)
	if caseResult.Code != http.StatusNoContent {
		t.Fatalf("revoke case status = %d", caseResult.Code)
	}
	if _, err := p.IssueTokenWithIdentity("new-worker", 60, identity.Identity{CaseID: "user-a", RunID: "case-2", ConnectionVersion: "v1", Provider: "openai"}); err == nil {
		t.Fatal("revoked case received another token")
	}
}
