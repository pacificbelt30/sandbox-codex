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
// The upstream sends one event, flushes, sleeps, then sends a second event.
// The client must receive the first event well before the upstream finishes.
func TestIntegration_AnthropicSSEStreaming(t *testing.T) {
	const gap = 400 * time.Millisecond
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fl, ok := w.(http.Flusher)
		if !ok {
			t.Error("upstream test server lacks Flusher")
			return
		}
		_, _ = io.WriteString(w, "event: message_start\ndata: {\"type\":\"message_start\"}\n\n")
		fl.Flush()
		time.Sleep(gap)
		_, _ = io.WriteString(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
		fl.Flush()
	}))
	defer upstream.Close()

	p := newAnthropicAPIKeyProxy(t, "sk-ant")
	p.anthropicUpstreamURL = upstream.URL
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	defer p.Stop()

	start := time.Now()
	resp := doAnthropicRequest(t, p, `{"stream":true}`, map[string]string{"x-api-key": "cdx-x"})
	defer func() { _ = resp.Body.Close() }()

	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q; want text/event-stream", ct)
	}

	reader := bufio.NewReader(resp.Body)
	firstLine, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("reading first SSE line: %v", err)
	}
	elapsed := time.Since(start)
	if !strings.Contains(firstLine, "message_start") {
		t.Errorf("first SSE line = %q; want message_start", firstLine)
	}
	// With incremental flushing the first event arrives almost immediately;
	// a buffered proxy would withhold it until the upstream stream completes
	// (>= gap). Allow generous slack to avoid flakiness.
	if elapsed >= gap {
		t.Errorf("first SSE event took %v (>= upstream gap %v): streaming is buffered, not incremental", elapsed, gap)
	}

	// The remainder of the stream must still arrive intact.
	rest, _ := io.ReadAll(reader)
	if !strings.Contains(string(rest), "message_stop") {
		t.Errorf("second SSE event missing from stream: %q", string(rest))
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
