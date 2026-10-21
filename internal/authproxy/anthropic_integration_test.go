package authproxy

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// doAnthropicRequest sends a request through the proxy's real listener and
// returns the response. The proxy is started and stopped by the caller.
func doAnthropicRequest(t *testing.T, p *Proxy, body string, headers map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		p.Endpoint()+"/anthropic/v1/messages", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	authorizeTestRequest(t, p, req)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request through proxy: %v", err)
	}
	return resp
}

// TestIntegration_AnthropicAPIKey_FullPath exercises the complete proxy listener
// (not just the handler) and asserts the upstream sees the real credential, the
// placeholder never leaks, hop-by-hop transforms are correct, and the body and
// path survive the round trip.
func TestIntegration_AnthropicAPIKey_FullPath(t *testing.T) {
	type captured struct {
		path, query, xAPIKey, auth, version, beta, body string
	}
	var got captured
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = captured{
			path:    r.URL.Path,
			query:   r.URL.RawQuery,
			xAPIKey: r.Header.Get("x-api-key"),
			auth:    r.Header.Get("Authorization"),
			version: r.Header.Get("anthropic-version"),
			beta:    r.Header.Get("anthropic-beta"),
			body:    string(b),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"type":"message","role":"assistant"}`))
	}))
	defer upstream.Close()

	p := newAnthropicAPIKeyProxy(t, "sk-ant-real-secret")
	p.anthropicUpstreamURL = upstream.URL
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	defer p.Stop()

	const reqBody = `{"model":"claude-3-5-haiku-latest","max_tokens":4,"messages":[{"role":"user","content":"hi"}]}`
	resp := doAnthropicRequest(t, p, reqBody, map[string]string{
		"Content-Type":      "application/json",
		"x-api-key":         "cdx-placeholder-from-container",
		"Authorization":     "Bearer cdx-placeholder-from-container",
		"anthropic-version": "2023-06-01",
	})
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	if got.path != "/v1/messages" {
		t.Errorf("upstream path = %q; want /v1/messages", got.path)
	}
	if got.xAPIKey != "sk-ant-real-secret" {
		t.Errorf("upstream x-api-key = %q; want injected real key", got.xAPIKey)
	}
	if strings.Contains(got.xAPIKey, "cdx-") {
		t.Errorf("placeholder leaked into x-api-key: %q", got.xAPIKey)
	}
	if got.auth != "" {
		t.Errorf("Authorization should be stripped in API-key mode, got %q", got.auth)
	}
	if got.version != "2023-06-01" {
		t.Errorf("anthropic-version = %q; want client value preserved", got.version)
	}
	if got.body != reqBody {
		t.Errorf("request body altered:\n got %q\nwant %q", got.body, reqBody)
	}
	if string(respBody) != `{"type":"message","role":"assistant"}` {
		t.Errorf("response body not relayed verbatim: %q", string(respBody))
	}
}

// TestIntegration_AnthropicOAuth_FullPath asserts the OAuth bearer path over the
// real listener: Authorization is set, x-api-key removed, beta header present.
func TestIntegration_AnthropicOAuth_FullPath(t *testing.T) {
	var gotXAPIKey, gotAuth, gotBeta string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotXAPIKey = r.Header.Get("x-api-key")
		gotAuth = r.Header.Get("Authorization")
		gotBeta = r.Header.Get("anthropic-beta")
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	p := newAnthropicOAuthProxy(t, "oat-real-bearer")
	p.anthropicUpstreamURL = upstream.URL
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	defer p.Stop()

	resp := doAnthropicRequest(t, p, `{}`, map[string]string{
		"x-api-key":      "cdx-placeholder",
		"anthropic-beta": "fine-grained-tool-streaming-2025-05-14",
	})
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)

	if gotAuth != "Bearer oat-real-bearer" {
		t.Errorf("Authorization = %q; want Bearer oat-real-bearer", gotAuth)
	}
	if gotXAPIKey != "" {
		t.Errorf("x-api-key should be stripped in OAuth mode, got %q", gotXAPIKey)
	}
	if !strings.Contains(gotBeta, anthropicOAuthBetaHeader) {
		t.Errorf("anthropic-beta = %q; want it to contain %q", gotBeta, anthropicOAuthBetaHeader)
	}
	// The client's own beta flag must be preserved alongside the OAuth one.
	if !strings.Contains(gotBeta, "fine-grained-tool-streaming-2025-05-14") {
		t.Errorf("anthropic-beta = %q; client beta flag was dropped", gotBeta)
	}
}

// TestIntegration_AnthropicSSEStreaming verifies that Server-Sent Events are
// relayed incrementally rather than buffered until the upstream stream ends.
// The upstream waits for the client to read the first event before sending the
// second. A buffered proxy cannot complete this handshake before the deadline.
func TestIntegration_AnthropicSSEStreaming(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	firstReceived := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Consume the POST before streaming, as the real Messages endpoint does.
		// Do not race the HTTP transport's request writer with an early response.
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			t.Error(err)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fl, ok := w.(http.Flusher)
		if !ok {
			t.Error("upstream test server lacks Flusher")
			return
		}
		_, _ = io.WriteString(w, "event: message_start\ndata: {\"type\":\"message_start\"}\n\n")
		fl.Flush()
		select {
		case <-firstReceived:
		case <-ctx.Done():
			return
		}
		_, _ = io.WriteString(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
		fl.Flush()
	}))
	defer upstream.Close()

	p := newAnthropicAPIKeyProxy(t, "sk-ant")
	p.anthropicUpstreamURL = upstream.URL
	// Keep the upstream pool independent from the downstream test client and
	// other httptest servers, whose Close drains the global default transport.
	p.httpClient = upstream.Client()
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	defer p.Stop()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.Endpoint()+"/anthropic/v1/messages", strings.NewReader(`{"stream":true}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("x-api-key", "cdx-x")
	authorizeTestRequest(t, p, req)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q; want text/event-stream", ct)
	}

	reader := bufio.NewReader(resp.Body)
	firstLine, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("reading first SSE line: %v", err)
	}
	if !strings.Contains(firstLine, "message_start") {
		t.Errorf("first SSE line = %q; want message_start", firstLine)
	}
	close(firstReceived)

	// The remainder of the stream must still arrive intact.
	rest, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("reading SSE completion: %v", err)
	}
	if !strings.Contains(string(rest), "message_stop") {
		t.Errorf("second SSE event missing from stream: %q", string(rest))
	}
}

// The client completes its upload only after receiving the first response
// event. Automatic HTTP/1 request draining would deadlock this exchange and can
// also close a request body still being read by the upstream transport writer.
func TestIntegration_HTTPResponseBeforeUploadCompletes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := http.NewResponseController(w).EnableFullDuplex(); err != nil {
			t.Error(err)
			return
		}
		first := make([]byte, 5)
		if _, err := io.ReadFull(r.Body, first); err != nil || string(first) != "first" {
			t.Errorf("initial upload: %q %v", first, err)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: ready\n\n")
		if err := http.NewResponseController(w).Flush(); err != nil {
			t.Error(err)
			return
		}
		rest, err := io.ReadAll(r.Body)
		if err != nil || string(rest) != "second" {
			t.Errorf("remaining upload: %q %v", rest, err)
			return
		}
		_, _ = io.WriteString(w, "data: complete\n\n")
	}))
	defer upstream.Close()
	p := newAnthropicAPIKeyProxy(t, "sk-ant")
	p.anthropicUpstreamURL, p.httpClient = upstream.URL, upstream.Client()
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	defer p.Stop()
	reader, writer := io.Pipe()
	defer func() { _ = reader.Close(); _ = writer.Close() }()
	ready := make(chan struct{})
	go func() {
		if _, err := io.WriteString(writer, "first"); err != nil {
			return
		}
		select {
		case <-ready:
			_, _ = io.WriteString(writer, "second")
			_ = writer.Close()
		case <-ctx.Done():
			_ = writer.CloseWithError(ctx.Err())
		}
	}()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.Endpoint()+"/anthropic/v1/messages", reader)
	if err != nil {
		t.Fatal(err)
	}
	authorizeTestRequest(t, p, req)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	stream := bufio.NewReader(resp.Body)
	line, err := stream.ReadString('\n')
	if err != nil || line != "data: ready\n" {
		t.Fatalf("first event before upload completion: %q %v", line, err)
	}
	close(ready)
	rest, err := io.ReadAll(stream)
	if err != nil || !strings.Contains(string(rest), "data: complete") {
		t.Fatalf("response after upload completion: %q %v", rest, err)
	}
}

// TestIntegration_AnthropicErrorPassthrough asserts non-2xx upstream responses
// (status + body) are relayed unchanged.
func TestIntegration_AnthropicErrorPassthrough(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"authentication_error"}}`))
	}))
	defer upstream.Close()

	p := newAnthropicAPIKeyProxy(t, "sk-ant")
	p.anthropicUpstreamURL = upstream.URL
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	defer p.Stop()

	resp := doAnthropicRequest(t, p, `{}`, map[string]string{"x-api-key": "cdx-x"})
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d; want 401", resp.StatusCode)
	}
	if !strings.Contains(string(body), "authentication_error") {
		t.Errorf("error body not relayed: %q", string(body))
	}
}
