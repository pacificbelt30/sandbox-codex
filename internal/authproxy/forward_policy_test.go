package authproxy

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/pacificbelt30/codex-dock/internal/networkpolicy"
)

func newEgressFixture(t *testing.T, rules networkpolicy.Rules) *Proxy {
	t.Helper()
	p, err := NewProxy(Config{Role: RoleEgress, ForwardPolicy: rules})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.httpClient.CloseIdleConnections() })
	return p
}

func TestHTTPRedirectCannotEscapeAllowlist(t *testing.T) {
	var deniedHits atomic.Int32
	denied := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { deniedHits.Add(1); w.WriteHeader(200) }))
	defer denied.Close()
	allowed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, denied.URL, http.StatusFound) }))
	defer allowed.Close()
	allowedURL := strings.Replace(allowed.URL, "127.0.0.1", "localhost", 1)
	p := newEgressFixture(t, networkpolicy.Rules{AllowDomains: []string{"localhost"}})
	rec := httptest.NewRecorder()
	p.handleForwardHTTP(rec, httptest.NewRequest(http.MethodGet, allowedURL, nil))
	if rec.Code != http.StatusForbidden || deniedHits.Load() != 0 {
		t.Fatalf("redirect escaped allowlist: code=%d hits=%d", rec.Code, deniedHits.Load())
	}
}

func TestPolicyChecksActualURLInsteadOfHostHeader(t *testing.T) {
	p := newEgressFixture(t, networkpolicy.Rules{AllowDomains: []string{"allowed.example"}})
	r := httptest.NewRequest(http.MethodGet, "http://denied.example/", nil)
	r.Host = "allowed.example"
	rec := httptest.NewRecorder()
	p.handleForwardHTTP(rec, r)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("Host header bypass: %d", rec.Code)
	}
}

func TestHTTPAndCONNECTEnforceSamePortsAndCIDRs(t *testing.T) {
	for _, rules := range []networkpolicy.Rules{{DenyPorts: []int{12345}}, {DenyCIDRs: []string{"127.0.0.0/8"}}} {
		p := newEgressFixture(t, rules)
		for _, method := range []string{http.MethodGet, http.MethodConnect} {
			rec := httptest.NewRecorder()
			r := httptest.NewRequest(method, "http://127.0.0.1:12345/", nil)
			if method == http.MethodConnect {
				r.Host = "127.0.0.1:12345"
				p.handleForwardConnect(rec, r)
			} else {
				p.handleForwardHTTP(rec, r)
			}
			if rec.Code != http.StatusForbidden {
				t.Fatalf("%s: %+v returned %d", method, rules, rec.Code)
			}
		}
	}
}

func TestAllowedCONNECTTunnelWithPolicy(t *testing.T) {
	backend := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "tunnel-ok") }))
	defer backend.Close()
	u, err := url.Parse(backend.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatal(err)
	}
	p := newEgressFixture(t, networkpolicy.Rules{AllowCIDRs: []string{"127.0.0.0/8"}, AllowPorts: []int{port}})
	proxy := httptest.NewServer(p.workerFacingHandler(http.NotFoundHandler()))
	defer proxy.Close()
	proxyURL, err := url.Parse(proxy.URL)
	if err != nil {
		t.Fatal(err)
	}
	certs := x509.NewCertPool()
	certs.AddCert(backend.Certificate())
	transport := &http.Transport{Proxy: http.ProxyURL(proxyURL), TLSClientConfig: &tls.Config{RootCAs: certs, MinVersion: tls.VersionTLS12}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport}
	resp, err := client.Get(backend.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil || string(body) != "tunnel-ok" {
		t.Fatalf("tunnel: %q %v", body, err)
	}
}

func TestInvalidPolicyRejectedBeforeServing(t *testing.T) {
	if _, err := NewProxy(Config{Role: RoleEgress, ForwardPolicy: networkpolicy.Rules{DenyCIDRs: []string{"invalid"}}}); err == nil {
		t.Fatal("invalid CIDR accepted")
	}
	if _, err := NewProxy(Config{Role: "typo"}); err == nil {
		t.Fatal("unknown role accepted")
	}
}
