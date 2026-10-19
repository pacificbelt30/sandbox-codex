package cxd

import (
	"context"
	"fmt"
	"github.com/pacificbelt30/codex-dock/internal/authproxy"
	"github.com/pacificbelt30/codex-dock/pkg/cxd/identity"
	"time"
)

type CredentialRoute = authproxy.CredentialRoute

// CredentialBroker runs on a caller-selected private address reachable by
// worker networks. Host network placement and firewall policy remain explicit.
type CredentialBroker struct{ proxy *authproxy.Proxy }

func StartCredentialBroker(listen, adminListen, adminSecret string, routes []CredentialRoute) (*CredentialBroker, error) {
	if listen == "" || adminListen == "" || len(adminSecret) < 32 {
		return nil, fmt.Errorf("broker needs explicit data/admin listeners and a private admin secret")
	}
	p, err := authproxy.NewProxy(authproxy.Config{ListenAddr: listen, AdminListenAddr: adminListen, AdminSecret: adminSecret, CredentialRoutes: routes, BrokerOnly: true, BlockPrivate: true})
	if err != nil {
		return nil, err
	}
	if err = p.Start(); err != nil {
		return nil, err
	}
	return &CredentialBroker{p}, nil
}
func (b *CredentialBroker) Endpoint() string { return b.proxy.Endpoint() }
func (b *CredentialBroker) Issue(worker string, ttl time.Duration, id identity.Identity) (string, error) {
	if ttl < time.Second || ttl > 24*time.Hour {
		return "", fmt.Errorf("broker token TTL must be in [1s,24h]")
	}
	return b.proxy.IssueTokenWithIdentity(worker, int(ttl/time.Second), id)
}
func (b *CredentialBroker) RevokeRun(ctx context.Context, id identity.Identity) error {
	return b.proxy.RevokeRun(ctx, id)
}
func (b *CredentialBroker) Close() { b.proxy.Stop() }
