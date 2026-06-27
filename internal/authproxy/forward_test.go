package authproxy

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestResolveAdminListenAddr(t *testing.T) {
	// Non-sentinel hosts are returned unchanged.
	for _, addr := range []string{"127.0.0.1:18081", "0.0.0.0:18081", "10.0.0.5:9000", "not-a-host-port"} {
		if got := resolveAdminListenAddr(addr); got != addr {
			t.Errorf("resolveAdminListenAddr(%q) = %q; want unchanged", addr, got)
		}
	}

	// The sentinel resolves to a concrete bind address: the egress IP when
	// detectable, otherwise the 0.0.0.0 fallback. Either way the host is no
	// longer the sentinel and the port is preserved.
	got := resolveAdminListenAddr(AdminBindEgress + ":18081")
	host, port, err := net.SplitHostPort(got)
	if err != nil {
		t.Fatalf("resolveAdminListenAddr sentinel produced invalid addr %q: %v", got, err)
	}
	if host == AdminBindEgress {
		t.Errorf("sentinel host %q was not resolved: %q", AdminBindEgress, got)
	}
	if port != "18081" {
		t.Errorf("port not preserved: got %q", port)
	}
	if ip := net.ParseIP(host); ip == nil {
		t.Errorf("resolved host %q is not an IP", host)
	}
}

func TestForwardAllowed(t *testing.T) {
	tests := []struct {
		name     string
		allow    []string
		hostport string
		want     bool
	}{
		{name: "empty allowlist allows all", allow: nil, hostport: "example.com:443", want: true},
		{name: "exact match", allow: []string{"github.com"}, hostport: "github.com:443", want: true},
		{name: "subdomain match", allow: []string{"github.com"}, hostport: "api.github.com:443", want: true},
		{name: "leading dot normalized", allow: []string{".npmjs.org"}, hostport: "registry.npmjs.org:443", want: true},
		{name: "no port", allow: []string{"pypi.org"}, hostport: "pypi.org", want: true},
		{name: "not allowed", allow: []string{"github.com"}, hostport: "evil.com:443", want: false},
		{name: "suffix trick not allowed", allow: []string{"github.com"}, hostport: "notgithub.com:443", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Proxy{cfg: Config{ForwardAllowDomains: tt.allow}}
			if got := p.forwardAllowed(tt.hostport); got != tt.want {
				t.Errorf("forwardAllowed(%q) = %v; want %v", tt.hostport, got, tt.want)
			}
		})
	}
}

func TestForwardHTTP_Proxies(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Backend", "yes")
		_, _ = io.WriteString(w, "hello from backend")
	}))
	defer backend.Close()

	p := &Proxy{cfg: Config{}, httpClient: http.DefaultClient}

	// Absolute-form request as an HTTP_PROXY client would send it.
	u, _ := url.Parse(backend.URL)
	req := httptest.NewRequest(http.MethodGet, backend.URL, nil)
	req.Host = u.Host
	rec := httptest.NewRecorder()
	p.handleForwardHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	if rec.Header().Get("X-Backend") != "yes" {
		t.Errorf("backend header not propagated: %v", rec.Header())
	}
	if body := rec.Body.String(); !strings.Contains(body, "hello from backend") {
		t.Errorf("body = %q; want backend content", body)
	}
}

func TestForwardHTTP_AllowlistDenied(t *testing.T) {
	p := &Proxy{cfg: Config{ForwardAllowDomains: []string{"github.com"}}, httpClient: http.DefaultClient}
	req := httptest.NewRequest(http.MethodGet, "http://evil.com/", nil)
	req.Host = "evil.com"
	rec := httptest.NewRecorder()
	p.handleForwardHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d; want 403 for disallowed host", rec.Code)
	}
}

func TestWorkerFacingHandler_RoutesConnectAndMux(t *testing.T) {
	muxHit := false
	mux := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		muxHit = true
		w.WriteHeader(http.StatusTeapot)
	})
	p := &Proxy{cfg: Config{ForwardAllowDomains: []string{"github.com"}}, httpClient: http.DefaultClient}
	h := p.workerFacingHandler(mux)

	// Origin-form request → data-plane mux.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/token", nil))
	if !muxHit || rec.Code != http.StatusTeapot {
		t.Errorf("origin-form request should reach mux (hit=%v code=%d)", muxHit, rec.Code)
	}

	// CONNECT to a disallowed host → 403 from the forward proxy, never the mux.
	muxHit = false
	rec = httptest.NewRecorder()
	connReq := httptest.NewRequest(http.MethodConnect, "http://evil.com:443", nil)
	connReq.Host = "evil.com:443"
	h.ServeHTTP(rec, connReq)
	if muxHit {
		t.Error("CONNECT should not reach the data-plane mux")
	}
	if rec.Code != http.StatusForbidden {
		t.Errorf("CONNECT to disallowed host = %d; want 403", rec.Code)
	}
}
