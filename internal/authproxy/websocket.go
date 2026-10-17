package authproxy

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// isWebSocketRequest returns true when the incoming request is a WebSocket upgrade.
func isWebSocketRequest(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket") &&
		strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade")
}

// handleWebSocketProxy tunnels a WebSocket connection to targetURLStr.
// It hijacks the incoming HTTP/1.1 connection, opens a raw TCP (or TLS) connection
// to the upstream, performs the HTTP upgrade handshake, then copies data
// bidirectionally between client and upstream.
func (p *Proxy) handleWebSocketProxy(w http.ResponseWriter, r *http.Request, targetURLStr string) {
	targetURL, err := url.Parse(targetURLStr)
	if err != nil {
		http.Error(w, "invalid target URL", http.StatusInternalServerError)
		return
	}

	host := targetURL.Host
	useTLS := targetURL.Scheme == "https" || targetURL.Scheme == "wss"
	if _, _, err := net.SplitHostPort(host); err != nil {
		if useTLS {
			host += ":443"
		} else {
			host += ":80"
		}
	}

	var upstreamConn net.Conn
	if useTLS {
		d := &tls.Dialer{Config: &tls.Config{ServerName: targetURL.Hostname()}}
		upstreamConn, err = d.DialContext(r.Context(), "tcp", host)
	} else {
		d := &net.Dialer{}
		upstreamConn, err = d.DialContext(r.Context(), "tcp", host)
	}
	if err != nil {
		http.Error(w, "connecting to upstream WebSocket: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer upstreamConn.Close() //nolint:errcheck

	// Send the HTTP upgrade request to the upstream, forwarding all client headers.
	// Authorization and ChatGPT-Account-Id are skipped here and replaced with
	// real host credentials below so placeholder tokens never reach the upstream.
	reqURI := targetURL.RequestURI()
	if _, err := fmt.Fprintf(upstreamConn, "%s %s HTTP/1.1\r\nHost: %s\r\n", r.Method, reqURI, targetURL.Host); err != nil {
		http.Error(w, "writing upgrade request: "+err.Error(), http.StatusBadGateway)
		return
	}
	for k, vv := range r.Header {
		if strings.EqualFold(k, "authorization") || strings.EqualFold(k, "chatgpt-account-id") {
			continue
		}
		for _, v := range vv {
			if _, err := fmt.Fprintf(upstreamConn, "%s: %s\r\n", k, v); err != nil {
				http.Error(w, "writing headers: "+err.Error(), http.StatusBadGateway)
				return
			}
		}
	}
	// Inject real host credentials.
	if p.oauthCreds != nil {
		p.mu.RLock()
		accessToken := p.oauthCreds.AccessToken
		accountID := p.oauthCreds.AccountID
		p.mu.RUnlock()
		if _, err := fmt.Fprintf(upstreamConn, "Authorization: Bearer %s\r\n", accessToken); err != nil {
			http.Error(w, "writing auth header: "+err.Error(), http.StatusBadGateway)
			return
		}
		if accountID != "" {
			if _, err := fmt.Fprintf(upstreamConn, "ChatGPT-Account-Id: %s\r\n", accountID); err != nil {
				http.Error(w, "writing account-id header: "+err.Error(), http.StatusBadGateway)
				return
			}
		}
	} else if p.apiKey != "" {
		if _, err := fmt.Fprintf(upstreamConn, "Authorization: Bearer %s\r\n", p.apiKey); err != nil {
			http.Error(w, "writing auth header: "+err.Error(), http.StatusBadGateway)
			return
		}
	}
	if _, err := fmt.Fprint(upstreamConn, "\r\n"); err != nil {
		http.Error(w, "writing header terminator: "+err.Error(), http.StatusBadGateway)
		return
	}

	// Read the upstream's 101 Switching Protocols response.
	upstreamBufReader := bufio.NewReader(upstreamConn)
	upstreamResp, err := http.ReadResponse(upstreamBufReader, r)
	if err != nil {
		http.Error(w, "reading upstream upgrade response: "+err.Error(), http.StatusBadGateway)
		return
	}
	if upstreamResp.StatusCode != http.StatusSwitchingProtocols {
		// Upstream rejected the upgrade; relay the error response to the client.
		for k, vv := range upstreamResp.Header {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(upstreamResp.StatusCode)
		_, _ = io.Copy(w, upstreamResp.Body)
		_ = upstreamResp.Body.Close()
		return
	}
	_ = upstreamResp.Body.Close()

	// Hijack the client connection to take over the raw TCP stream.
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "WebSocket proxying requires HTTP/1.1", http.StatusInternalServerError)
		return
	}
	clientConn, clientBufRW, err := hj.Hijack()
	if err != nil {
		log.Printf("WebSocket hijack error: %v", err)
		return
	}
	defer clientConn.Close() //nolint:errcheck

	// Forward the 101 response to the client.
	if _, err := fmt.Fprint(clientBufRW, "HTTP/1.1 101 Switching Protocols\r\n"); err != nil {
		log.Printf("WebSocket: writing 101 to client: %v", err)
		return
	}
	for k, vv := range upstreamResp.Header {
		for _, v := range vv {
			if _, err := fmt.Fprintf(clientBufRW, "%s: %s\r\n", k, v); err != nil {
				log.Printf("WebSocket: writing 101 headers to client: %v", err)
				return
			}
		}
	}
	if _, err := fmt.Fprint(clientBufRW, "\r\n"); err != nil {
		log.Printf("WebSocket: writing 101 terminator to client: %v", err)
		return
	}
	if err := clientBufRW.Flush(); err != nil {
		log.Printf("WebSocket: flushing 101 to client: %v", err)
		return
	}

	if p.cfg.Verbose {
		fmt.Printf("WebSocket tunnel established: %s <-> %s\n", r.URL.Path, targetURLStr)
	}

	// Bidirectional tunnel.
	// upstreamBufReader drains any bytes already buffered after the 101 headers
	// before falling through to raw reads from upstreamConn.
	errc := make(chan error, 2)
	go func() {
		_, err := io.Copy(clientConn, upstreamBufReader)
		errc <- err
	}()
	go func() {
		_, err := io.Copy(upstreamConn, clientConn)
		errc <- err
	}()
	<-errc
}
