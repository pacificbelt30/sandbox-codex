package authproxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

// workerFacingHandler returns the HTTP handler installed on the worker-facing
// listener. Behaviour depends on the role:
//
//   - RoleEgress: CONNECT tunnels and absolute-form HTTP requests (sent by clients
//     honoring HTTP(S)_PROXY) are handled by the forward proxy; everything else
//     falls through to mux (just /health).
//   - RoleAuth: this instance does NOT forward general traffic. CONNECT and
//     absolute-form requests are rejected (405); origin-form requests reach the
//     reverse-proxy / token mux.
func (p *Proxy) workerFacingHandler(mux http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		isForward := r.Method == http.MethodConnect || (r.URL.IsAbs() && r.URL.Host != "")
		if isForward {
			if !p.cfg.isEgress() {
				http.Error(w, "this proxy does not forward general traffic; use the egress proxy", http.StatusMethodNotAllowed)
				return
			}
			if r.Method == http.MethodConnect {
				p.handleForwardConnect(w, r)
			} else {
				p.handleForwardHTTP(w, r)
			}
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

	destConn, err := p.guardedDialContext(r.Context(), "tcp", host)
	if err != nil {
		if isBlockedDestErr(err) {
			http.Error(w, err.Error(), http.StatusForbidden)
		} else {
			http.Error(w, err.Error(), http.StatusBadGateway)
		}
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
		if isBlockedDestErr(err) {
			http.Error(w, err.Error(), http.StatusForbidden)
		} else {
			http.Error(w, err.Error(), http.StatusBadGateway)
		}
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

// blockedDestErr marks a dial refused because the destination resolves to a
// private/LAN range. Surfaced as 403 rather than 502.
type blockedDestErr struct{ msg string }

func (e *blockedDestErr) Error() string { return e.msg }

func isBlockedDestErr(err error) bool {
	var b *blockedDestErr
	return errors.As(err, &b)
}

// guardedDialContext resolves addr and, when BlockPrivate is set, refuses any
// destination that resolves to a private/loopback/link-local address (stopping a
// worker from pivoting into the host LAN or cloud metadata). It then dials a
// resolved IP directly (pinning it, so a later DNS rebind cannot redirect the
// connection). When BlockPrivate is off it dials normally.
func (p *Proxy) guardedDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	d := &net.Dialer{Timeout: 30 * time.Second}
	if !p.cfg.BlockPrivate {
		return d.DialContext(ctx, network, addr)
	}

	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ips, err := screenDest(ctx, host)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for _, ip := range ips {
		conn, derr := d.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if derr == nil {
			return conn, nil
		}
		lastErr = derr
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no address for %s", host)
	}
	return nil, lastErr
}

// screenDest resolves host and returns its IPs, or a blockedDestErr if any
// resolved address is private/loopback/link-local (anti-rebinding: reject the
// whole host if any answer is internal). IP literals resolve without DNS.
func screenDest(ctx context.Context, host string) ([]net.IP, error) {
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	ips := make([]net.IP, 0, len(addrs))
	for _, a := range addrs {
		if isBlockedIP(a.IP) {
			return nil, &blockedDestErr{msg: fmt.Sprintf("destination %s (%s) is in a blocked private/LAN range", host, a.IP)}
		}
		ips = append(ips, a.IP)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("no address for %s", host)
	}
	return ips, nil
}

// isBlockedIP reports whether ip is in a range a worker must not reach via the
// egress proxy: loopback, private (RFC1918 + ULA), link-local (incl. the cloud
// metadata 169.254.169.254), CGNAT (100.64/10), and the unspecified address.
func isBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return true
	}
	// Carrier-grade NAT 100.64.0.0/10 is not covered by IsPrivate.
	if v4 := ip.To4(); v4 != nil && v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
		return true
	}
	return false
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
