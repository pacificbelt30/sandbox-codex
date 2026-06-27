package authproxy

import (
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

// workerFacingHandler returns the HTTP handler installed on the worker-facing
// listener. It turns the auth proxy into a router: HTTP CONNECT tunnels and
// absolute-form HTTP requests (sent by clients honoring HTTP(S)_PROXY) are
// handled by the forward proxy, while origin-form requests (the credential
// injecting reverse-proxy routes and token endpoints) fall through to mux.
func (p *Proxy) workerFacingHandler(mux http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodConnect {
			p.handleForwardConnect(w, r)
			return
		}
		// A forward-proxy HTTP request carries an absolute URI (scheme + host).
		if r.URL.IsAbs() && r.URL.Host != "" {
			p.handleForwardHTTP(w, r)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

// handleForwardConnect tunnels an HTTPS (or any TCP) connection to the requested
// host after an allowlist check. The proxy never sees the tunneled bytes (opaque
// TLS), which is why credential-injecting API traffic uses the reverse-proxy
// routes instead; this path carries general egress (git, npm, pip, curl, ...).
func (p *Proxy) handleForwardConnect(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	if host == "" {
		host = r.URL.Host
	}
	if !p.forwardAllowed(host) {
		http.Error(w, "destination not allowed by forward-proxy allowlist", http.StatusForbidden)
		return
	}

	dialer := &net.Dialer{Timeout: 30 * time.Second}
	destConn, err := dialer.DialContext(r.Context(), "tcp", host)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "forward proxy requires HTTP/1.1", http.StatusInternalServerError)
		_ = destConn.Close()
		return
	}
	clientConn, _, err := hj.Hijack()
	if err != nil {
		log.Printf("forward proxy hijack error: %v", err)
		_ = destConn.Close()
		return
	}

	if _, err := clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
		_ = clientConn.Close()
		_ = destConn.Close()
		return
	}

	// Splice the two connections together until either side closes.
	go func() {
		_, _ = io.Copy(destConn, clientConn)
		_ = destConn.Close()
		_ = clientConn.Close()
	}()
	go func() {
		_, _ = io.Copy(clientConn, destConn)
		_ = clientConn.Close()
		_ = destConn.Close()
	}()
}

// handleForwardHTTP forwards a plain (non-CONNECT) HTTP request received in
// absolute form to its upstream and streams the response back to the worker.
func (p *Proxy) handleForwardHTTP(w http.ResponseWriter, r *http.Request) {
	if !p.forwardAllowed(r.Host) {
		http.Error(w, "destination not allowed by forward-proxy allowlist", http.StatusForbidden)
		return
	}

	outReq := r.Clone(r.Context())
	outReq.RequestURI = ""
	// Hop-by-hop proxy header is meaningless upstream.
	outReq.Header.Del("Proxy-Connection")

	resp, err := p.httpClient.Do(outReq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// forwardAllowed reports whether the destination host is permitted by the
// forward-proxy allowlist. An empty allowlist permits everything.
func (p *Proxy) forwardAllowed(hostport string) bool {
	if len(p.cfg.ForwardAllowDomains) == 0 {
		return true
	}
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for _, d := range p.cfg.ForwardAllowDomains {
		d = strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(d), "."), "."))
		if d == "" {
			continue
		}
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}
