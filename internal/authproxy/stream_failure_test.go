package authproxy

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

type failedStream struct{ sent bool }

func (s *failedStream) Read(p []byte) (int, error) {
	if !s.sent {
		s.sent = true
		return copy(p, "data: started\n\n"), nil
	}
	return 0, io.ErrUnexpectedEOF
}

func TestFailedUpstreamStreamDoesNotBecomeCleanEOF(t *testing.T) {
	for _, route := range []string{"openai", "anthropic", "broker", "egress"} {
		t.Run(route, func(t *testing.T) {
			// A broken chunked upstream has no Content-Length that the worker
			// could independently use to discover truncation.
			handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "data: started\n\n")
				_ = http.NewResponseController(w).Flush()
				panic(http.ErrAbortHandler)
			})
			var upstream *httptest.Server
			if route == "broker" {
				upstream = httptest.NewTLSServer(handler)
			} else {
				upstream = httptest.NewServer(handler)
			}
			defer upstream.Close()
			p := newTestProxy(t)
			p.apiUpstreamURL, p.anthropicUpstreamURL = upstream.URL, upstream.URL
			p.httpClient = upstream.Client()
			p.anthropicAPIKey = "fixture-only"
			path := "/v1/responses"
			switch route {
			case "anthropic":
				path = "/anthropic/v1/messages"
			case "broker":
				p.cfg.CredentialRoutes = []CredentialRoute{{Name: "fixture", Upstream: upstream.URL, Paths: []string{"/events"}, Methods: []string{"GET"}, Header: "X-Api-Key", Secret: func(_ context.Context) (string, error) { return "fixture-only", nil }}}
				path = "/broker/fixture/events"
			case "egress":
				var err error
				p, err = NewProxy(Config{Role: RoleEgress})
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := p.Start(); err != nil {
				t.Fatal(err)
			}
			defer p.Stop()
			req, err := http.NewRequest(http.MethodGet, p.Endpoint()+path, nil)
			if err != nil {
				t.Fatal(err)
			}
			if route == "egress" {
				req.URL, err = url.Parse(upstream.URL)
				if err != nil {
					t.Fatal(err)
				}
				req.Host = req.URL.Host
				proxyURL, _ := url.Parse(p.Endpoint())
				transport := &http.Transport{Proxy: http.ProxyURL(proxyURL)}
				defer transport.CloseIdleConnections()
				client := &http.Client{Transport: transport}
				assertBrokenStream(t, client, req)
				return
			}
			authorizeTestRequest(t, p, req)
			assertBrokenStream(t, http.DefaultClient, req)
		})
	}
}

func assertBrokenStream(t *testing.T, client *http.Client, req *http.Request) {
	t.Helper()
	resp, err := client.Do(req)
	if err != nil {
		// Without an immediate flush the failed response may be reset before
		// headers reach the client; it must still never look successful.
		if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatal("unexpected request failure:", err)
		}
		return
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || err == nil || !strings.Contains(string(data), "started") {
		t.Fatalf("truncated stream appeared complete: status=%d data=%q err=%v", resp.StatusCode, data, err)
	}
}

func TestStreamingCopyPreservesReadErrorsAndFinalBytes(t *testing.T) {
	w := httptest.NewRecorder()
	if err := copyAndFlush(w, &failedStream{}); !errors.Is(err, io.ErrUnexpectedEOF) || w.Body.String() != "data: started\n\n" {
		t.Fatalf("copy swallowed failure: body=%q err=%v", w.Body.String(), err)
	}
	w = httptest.NewRecorder()
	if err := copyAndFlush(w, strings.NewReader("complete")); err != nil || w.Body.String() != "complete" {
		t.Fatalf("successful copy: body=%q err=%v", w.Body.String(), err)
	}
}
