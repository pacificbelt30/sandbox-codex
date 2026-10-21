package authproxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pacificbelt30/codex-dock/pkg/cxd/identity"
)

func TestTokenLookupLifecycle(t *testing.T) {
	for _, action := range []string{"replace", "restore", "restore-same", "revoke", "expire", "run", "case", "stop"} {
		t.Run(action, func(t *testing.T) {
			p := newTestProxy(t)
			t.Cleanup(p.Stop)
			id := identity.Identity{CaseID: "fixture", RunID: "run", ConnectionVersion: "v1", Provider: "openai"}
			token, err := p.IssueTokenWithIdentity("worker", 60, id)
			if err != nil {
				t.Fatal(err)
			}
			request := func(value string) *http.Request {
				r := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
				r.Header.Set("Authorization", "Bearer "+value)
				return r
			}
			active, release, ok := p.workerRequest(request(token), false)
			if !ok {
				t.Fatal("issued token rejected")
			}
			defer release()
			var replacement string
			switch action {
			case "replace":
				replacement, err = p.IssueTokenWithIdentity("worker", 60, id)
			case "restore", "restore-same":
				replacement = token
				if action == "restore" {
					replacement, err = generateToken()
				}
				if err == nil {
					err = p.RestoreTokenWithIdentity("worker", replacement, 60, id)
				}
			case "revoke":
				p.RevokeToken("worker")
			case "expire":
				p.expireTokens(time.Now().Add(2 * time.Minute))
			case "run":
				err = p.RevokeRun(context.Background(), id)
			case "case":
				p.revokeCaseTokens()
			case "stop":
				p.Stop()
			}
			if err != nil {
				t.Fatal(err)
			}
			select {
			case <-active.Context().Done():
			case <-time.After(time.Second):
				t.Fatal("old stream survived lease replacement/removal")
			}
			_, cleanup, accepted := p.workerRequest(request(token), false)
			if cleanup != nil {
				cleanup()
			}
			if accepted != (action == "restore-same") {
				t.Fatal("old token authorization did not match its current lease")
			}
			if replacement != "" {
				_, cleanup, accepted = p.workerRequest(request(replacement), false)
				if !accepted {
					t.Fatal("replacement lease rejected")
				}
				cleanup()
				if err := p.RestoreToken("other-worker", replacement, 60); err == nil {
					t.Fatal("active token accepted for another worker")
				}
			}
			p.mu.RLock()
			defer p.mu.RUnlock()
			if len(p.tokensByValue) != len(p.tokens) {
				t.Fatal("removed token retained by lookup index")
			}
		})
	}
}
