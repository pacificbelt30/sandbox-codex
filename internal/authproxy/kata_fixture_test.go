package authproxy

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"syscall"

	"github.com/pacificbelt30/codex-dock/pkg/cxd/credentialstore"
)

// This entrypoint exists only in the opt-in test executable, never in cxd.
// The fixture image replaces /codex-dock with `go test -c ./internal/authproxy`.
// Production proxy handlers and credential-store calls remain unchanged; only
// fixed upstream endpoints are redirected to synthetic loopback HTTP servers.
func runKataFixtureProxy() int {
	if len(os.Args) < 3 || os.Args[1] != "proxy" || os.Args[2] != "serve" {
		return 2
	}
	f := flag.NewFlagSet("fixture-proxy", flag.ContinueOnError)
	var cfg Config
	var socket, role string
	var block bool
	f.StringVar(&cfg.ListenAddr, "listen", "", "")
	f.StringVar(&cfg.AdminListenAddr, "admin-listen", "", "")
	f.StringVar(&cfg.AdminSecret, "admin-secret", "", "")
	f.StringVar(&cfg.CaseID, "case-id", "", "")
	f.StringVar(&socket, "credential-store-socket", "", "")
	f.StringVar(&role, "role", "auth", "")
	f.BoolVar(&block, "block-private", false, "")
	f.Func("allow-model", "", func(v string) error { cfg.AllowedModels = append(cfg.AllowedModels, v); return nil })
	if err := f.Parse(os.Args[3:]); err != nil {
		return 2
	}
	store, err := credentialstore.NewUnixClient(socket)
	if err != nil {
		return 1
	}
	cfg.CredentialStore = store
	p, err := NewProxy(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fixture proxy initialization:", err)
		return 1
	}
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"access_token":"fixture-access-rotated","refresh_token":"fixture-refresh-rotated"}`)
			return
		}
		var body struct {
			Stream bool `json:"stream"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: {\"fixture\":true}\n\n")
			w.(http.Flusher).Flush()
			<-r.Context().Done()
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer up.Close()
	p.oauthTokenURL = up.URL + "/oauth/token"
	p.apiUpstreamURL, p.chatgptURL, p.anthropicUpstreamURL = up.URL, up.URL, up.URL
	if err := p.Start(); err != nil {
		return 1
	}
	defer p.Stop()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	<-ctx.Done()
	return 0
}
