package authproxy

import (
	"context"
	"encoding/pem"
	"github.com/pacificbelt30/codex-dock/internal/networkpolicy"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestChainedProxyReceivesOnlyVerifiedIP(t *testing.T) {
	var hits atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Method != "CONNECT" || r.Host != "8.8.8.8:443" {
			t.Errorf("unverified tunnel: %s %s", r.Method, r.Host)
		}
		w.WriteHeader(403)
	}))
	defer proxy.Close()
	p, err := NewProxy(Config{Role: RoleEgress, BlockPrivate: true, UpstreamProxyURL: proxy.URL})
	if err != nil {
		t.Fatal(err)
	}
	_, err = (networkpolicy.Dialer{Policy: p.egressPolicy, Connect: p.connectVerified}).DialContext(context.Background(), "tcp", "8.8.8.8:443")
	if err == nil || hits.Load() != 1 {
		t.Fatalf("%v %d", err, hits.Load())
	}
	_, err = (networkpolicy.Dialer{Policy: p.egressPolicy, Connect: p.connectVerified}).DialContext(context.Background(), "tcp", "127.0.0.1:443")
	if !isBlockedDestErr(err) || hits.Load() != 1 {
		t.Fatal("private address reached chained proxy")
	}
}
func TestDenialEventsExcludeRequestSecrets(t *testing.T) {
	p, err := NewProxy(Config{Role: RoleEgress, ForwardPolicy: networkpolicy.Rules{DenyDomains: []string{"blocked.example"}}})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "http://blocked.example/private?token=never-record", nil)
	r.Header.Set("Authorization", "Bearer never-record")
	w := httptest.NewRecorder()
	p.handleForwardHTTP(w, r)
	events := p.PolicyEvents(0)
	if w.Code != 403 || w.Header().Get("X-Cxd-Policy-Reason") != "domain_denied" || len(events) != 1 || events[0].Destination != "blocked.example:80" {
		t.Fatalf("%d %v", w.Code, events)
	}
}

func TestCustomCARootsAndWebSocketPrivateGuard(t *testing.T) {
	var hits atomic.Int32
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1); w.WriteHeader(http.StatusOK) }))
	defer upstream.Close()
	bundle := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: upstream.Certificate().Raw})
	p, err := NewProxy(Config{Role: RoleEgress, CABundlePEM: bundle})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	p.handleForwardHTTP(w, httptest.NewRequest("GET", upstream.URL, nil))
	if w.Code != 200 || hits.Load() != 1 {
		t.Fatalf("custom CA did not verify: %d %s", w.Code, w.Body.String())
	}
	guarded, err := NewProxy(Config{Role: RoleEgress, BlockPrivate: true, CABundlePEM: bundle})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/v1/stream", nil)
	w = httptest.NewRecorder()
	guarded.handleWebSocketProxy(w, r, upstream.URL, func(http.Header) {})
	if w.Code != http.StatusBadGateway || hits.Load() != 1 {
		t.Fatal("WebSocket bypassed private-address guard")
	}
	if _, err = NewProxy(Config{Role: RoleEgress, CABundlePEM: []byte("not a certificate")}); err == nil {
		t.Fatal("invalid CA accepted")
	}
}
