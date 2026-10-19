package authproxy

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Proxy chaining connects only to an IP already checked by networkpolicy. The
// trusted upstream proxy receives CONNECT <verified-IP>:port, never a hostname
// it could resolve differently. This also works for plain HTTP and SSH tunnels.
func (p *Proxy) connectVerified(ctx context.Context, network, address string) (net.Conn, error) {
	if p.upstreamProxy == nil {
		return (&net.Dialer{Timeout: 30 * time.Second}).DialContext(ctx, network, address)
	}
	u := p.upstreamProxy
	endpoint := u.Host
	if u.Port() == "" {
		port := "80"
		if u.Scheme == "https" {
			port = "443"
		}
		endpoint = net.JoinHostPort(u.Hostname(), port)
	}
	conn, err := (&net.Dialer{Timeout: 30 * time.Second}).DialContext(ctx, "tcp", endpoint)
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			_ = conn.Close()
		}
	}()
	rawConn := conn
	stop := context.AfterFunc(ctx, func() { _ = rawConn.Close() })
	defer stop()
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	if u.Scheme == "https" {
		secure := tls.Client(conn, &tls.Config{ServerName: u.Hostname(), RootCAs: p.rootCAs, MinVersion: tls.VersionTLS12})
		if err = secure.HandshakeContext(ctx); err != nil {
			return nil, err
		}
		conn = secure
	}
	req := &http.Request{Method: http.MethodConnect, URL: &url.URL{Opaque: address}, Host: address, Header: make(http.Header)}
	if err = req.Write(conn); err != nil {
		return nil, err
	}
	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, req) //nolint:bodyclose // Successful CONNECT transfers ownership to the returned tunnel; closing its unbounded Body would drain the stream.
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		_ = conn.Close()
		_ = resp.Body.Close()
		return nil, fmt.Errorf("configured upstream proxy rejected CONNECT (HTTP %d)", resp.StatusCode)
	}
	_ = conn.SetDeadline(time.Time{})
	ok = true
	return &bufferedTunnel{Conn: conn, reader: reader}, nil
}

type bufferedTunnel struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedTunnel) Read(p []byte) (int, error) { return c.reader.Read(p) }
func (p *Proxy) configureUpstream() error {
	if len(p.cfg.CABundlePEM) > 0 {
		roots, err := x509.SystemCertPool()
		if err != nil {
			roots = x509.NewCertPool()
		}
		if !roots.AppendCertsFromPEM(p.cfg.CABundlePEM) {
			return fmt.Errorf("custom CA bundle has no certificates")
		}
		p.rootCAs = roots
	}
	if p.cfg.UpstreamProxyURL != "" {
		u, err := url.Parse(p.cfg.UpstreamProxyURL)
		if err != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") || u.RawQuery != "" || u.Fragment != "" || strings.Trim(u.Path, "/") != "" {
			return fmt.Errorf("upstream proxy must be an HTTP(S) origin without credentials")
		}
		p.upstreamProxy = u
	}
	return nil
}
