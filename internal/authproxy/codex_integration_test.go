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

// doProxyRequest sends a request through the proxy's real listener.
func doProxyRequest(t *testing.T, p *Proxy, method, path, body string, headers map[string]string) *http.Response {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, p.Endpoint()+path, r)
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

// TestIntegration_CodexAPIKey_FullPath runs the real proxy listener and asserts
// the API-key /v1 path: routing to api.openai.com/v1, real key injection, the
// container placeholder not leaking, and the response relayed.
func TestIntegration_CodexAPIKey_FullPath(t *testing.T) {
	var gotPath, gotAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[]}`))
	}))
	defer upstream.Close()

	p := newTestProxy(t) // apiKey = sk-test-key-12345
	p.apiUpstreamURL = upstream.URL
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	defer p.Stop()

	resp := doProxyRequest(t, p, http.MethodGet, "/v1/models", "", map[string]string{
		"Authorization": "Bearer cdx-placeholder-from-container",
	})
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	if gotPath != "/models" {
		t.Errorf("upstream path = %q; want /models", gotPath)
	}
	if gotAuth != "Bearer sk-test-key-12345" {
		t.Errorf("upstream Authorization = %q; want injected real key", gotAuth)
	}
	if strings.Contains(gotAuth, "cdx-") {
		t.Errorf("placeholder leaked into Authorization: %q", gotAuth)
	}
	if !strings.Contains(string(body), `"object":"list"`) {
		t.Errorf("response body not relayed: %q", string(body))
	}
}

// TestIntegration_CodexOAuth_FullPath asserts the OAuth path over the real
// listener: /v1/* routes to the ChatGPT backend /codex/*, the real access token
// is injected as a Bearer, and ChatGPT-Account-Id is set from host creds.
func TestIntegration_CodexOAuth_FullPath(t *testing.T) {
	var gotPath, gotAuth, gotAccountID string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotAccountID = r.Header.Get("ChatGPT-Account-Id")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"resp_1"}`))
	}))
	defer upstream.Close()

	p := newOAuthTestProxy(t, "oat-real-access")
	p.oauthCreds.AccountID = "acct-real-int"
	p.chatgptURL = upstream.URL // handler appends /codex
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	defer p.Stop()

	resp := doProxyRequest(t, p, http.MethodPost, "/v1/responses", `{"model":"gpt-5-codex"}`, map[string]string{
		"Content-Type":       "application/json",
		"Authorization":      "Bearer cdx-placeholder",
		"ChatGPT-Account-Id": "acct-wrong-from-container",
	})
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)

	if gotPath != "/codex/responses" {
		t.Errorf("upstream path = %q; want /codex/responses", gotPath)
	}
	if gotAuth != "Bearer oat-real-access" {
		t.Errorf("upstream Authorization = %q; want Bearer oat-real-access", gotAuth)
	}
	if gotAccountID != "acct-real-int" {
		t.Errorf("ChatGPT-Account-Id = %q; want acct-real-int (host value overrides container)", gotAccountID)
	}
}

// TestIntegration_CodexSSEStreaming verifies the OpenAI /v1 path streams SSE
// incrementally (the copyAndFlush behaviour applies to both providers).
func TestIntegration_CodexSSEStreaming(t *testing.T) {
	const gap = 400 * time.Millisecond
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fl := w.(http.Flusher)
		_, _ = io.WriteString(w, "event: response.created\ndata: {}\n\n")
		fl.Flush()
		time.Sleep(gap)
		_, _ = io.WriteString(w, "event: response.completed\ndata: {}\n\n")
		fl.Flush()
	}))
	defer upstream.Close()

	p := newTestProxy(t)
	p.apiUpstreamURL = upstream.URL
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	defer p.Stop()

	start := time.Now()
	resp := doProxyRequest(t, p, http.MethodPost, "/v1/responses", `{"stream":true}`, map[string]string{
		"Authorization": "Bearer cdx-x",
	})
	defer func() { _ = resp.Body.Close() }()

	reader := bufio.NewReader(resp.Body)
	firstLine, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("reading first SSE line: %v", err)
	}
	elapsed := time.Since(start)
	if !strings.Contains(firstLine, "response.created") {
		t.Errorf("first SSE line = %q; want response.created", firstLine)
	}
	if elapsed >= gap {
		t.Errorf("first SSE event took %v (>= upstream gap %v): streaming is buffered", elapsed, gap)
	}
	rest, _ := io.ReadAll(reader)
	if !strings.Contains(string(rest), "response.completed") {
		t.Errorf("second SSE event missing: %q", string(rest))
	}
}
