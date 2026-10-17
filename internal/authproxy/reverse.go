package authproxy

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// reverseProxy strips stripPrefix from r.URL.Path, appends it to targetBase,
// and forwards the request upstream, copying status, headers, and body back.
// inject overwrites auth headers with the host's real credentials before the
// request leaves the proxy.
func (p *Proxy) reverseProxy(w http.ResponseWriter, r *http.Request, stripPrefix, targetBase string, inject func(http.Header)) {
	authenticated, done, ok := p.workerRequest(r, stripPrefix == "/anthropic")
	if !ok {
		http.Error(w, "worker token invalid or expired", http.StatusUnauthorized)
		return
	}
	defer done()
	r = authenticated
	path := strings.TrimPrefix(r.URL.Path, stripPrefix)
	target := targetBase + path
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}

	// WebSocket upgrade requests require raw TCP tunneling, not HTTP proxying.
	if isWebSocketRequest(r) {
		p.handleWebSocketProxy(w, r, target, inject)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "reading request body", http.StatusBadRequest)
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), r.Method, target, bytes.NewReader(body))
	if err != nil {
		http.Error(w, "creating upstream request", http.StatusInternalServerError)
		return
	}

	// Copy headers, skipping hop-by-hop headers.
	hopByHop := map[string]bool{
		"connection":          true,
		"keep-alive":          true,
		"proxy-authenticate":  true,
		"proxy-authorization": true,
		"te":                  true,
		"trailers":            true,
		"transfer-encoding":   true,
		"upgrade":             true,
	}
	for k, vv := range r.Header {
		if hopByHop[strings.ToLower(k)] || strings.EqualFold(k, "X-Codex-Token") || strings.EqualFold(k, "Authorization") || strings.EqualFold(k, "X-Api-Key") {
			continue
		}
		for _, v := range vv {
			req.Header.Add(k, v)
		}
	}
	// Inject real host credentials, overriding any placeholder the container sent.
	inject(req.Header)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		http.Error(w, "upstream error: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	copyAndFlush(w, resp.Body)

	if p.cfg.Verbose {
		fmt.Printf("Proxied %s %s -> %s (%d)\n", r.Method, r.URL.Path, target, resp.StatusCode)
	}
}

// copyAndFlush streams src to w, flushing after every chunk when w supports
// http.Flusher. This preserves real-time delivery of Server-Sent Events
// (text/event-stream), which both the Anthropic Messages API and the OpenAI
// Responses API use for streaming responses. A plain io.Copy would let the
// net/http response buffer hold events until it fills or the handler returns,
// stalling interactive agents.
func copyAndFlush(w http.ResponseWriter, src io.Reader) {
	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 32*1024)
	for {
		n, rerr := src.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if rerr != nil {
			return
		}
	}
}
