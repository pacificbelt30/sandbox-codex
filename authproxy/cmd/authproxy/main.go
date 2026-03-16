// Command authproxy runs the auth proxy server as a standalone process.
//
// Usage:
//
//	authproxy [flags]
//
// Flags:
//
//	-listen string       listen address (default "0.0.0.0:18080")
//	-token-ttl int       default token TTL in seconds (default 3600)
//	-admin-secret string shared secret for /admin/* endpoints (default: none)
//	-verbose             log every proxied request
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/pacificbelt30/authproxy"
)

func main() {
	listen := flag.String("listen", "0.0.0.0:18080", "listen address (host:port)")
	tokenTTL := flag.Int("token-ttl", 3600, "default token TTL in seconds")
	adminSecret := flag.String("admin-secret", "", "shared secret for /admin/* endpoints (empty = no auth)")
	verbose := flag.Bool("verbose", false, "log every proxied request")
	flag.Parse()

	p, err := authproxy.NewProxy(authproxy.Config{
		ListenAddr:  *listen,
		TokenTTL:    *tokenTTL,
		AdminSecret: *adminSecret,
		Verbose:     *verbose,
	})
	if err != nil {
		log.Fatalf("authproxy: init failed: %v", err)
	}

	if err := p.Start(); err != nil {
		log.Fatalf("authproxy: start failed: %v", err)
	}

	fmt.Printf("authproxy listening on %s\n", p.Endpoint())
	fmt.Printf("container endpoint: %s\n", p.ContainerEndpoint())

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	fmt.Println("authproxy: shutting down")
	p.Stop()
}
