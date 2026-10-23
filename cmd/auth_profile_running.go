package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/pacificbelt30/codex-dock/internal/authproxy"
	"github.com/pacificbelt30/codex-dock/internal/engine"
	"github.com/pacificbelt30/codex-dock/pkg/cxd/credentialstore"
	"github.com/spf13/viper"
)

const proxyVaultLabel = "io.codex-dock.encrypted-profile"
const proxyProfileNameLabel = "io.codex-dock.profile-name"
const proxyVaultProtocolLabel = "io.codex-dock.encrypted-store"
const proxyVaultProtocolVersion = "stdin-key-v1"

// Reuse requires both authenticated proxy metadata and the exact local wrapped
// key fingerprint. A changed connection/key cannot reuse an old unlocked proxy.
func runningEncryptedProfile(ctx context.Context, name string) (proxyAuthState, string, bool) {
	state, connection, _, ok := runningEncryptedProfileDetails(ctx, name)
	return state, connection, ok
}
func runningEncryptedProfileDetails(ctx context.Context, name string) (proxyAuthState, string, bool, bool) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	s, err := openAuthProfile(name)
	if err != nil {
		return proxyAuthState{}, "", false, false
	}
	encrypted, ok := s.(*credentialstore.EncryptedStore)
	if !ok {
		return proxyAuthState{}, "", false, false
	}
	info, err := encrypted.Info(ctx)
	if err != nil {
		return proxyAuthState{}, "", false, false
	}
	c, err := engine.NewClient()
	if err != nil {
		return proxyAuthState{}, "", false, false
	}
	defer func() { _ = c.Close() }()
	v, err := c.ContainerInspect(ctx, proxyRunName)
	if err != nil || v.Config == nil || v.State == nil || !v.State.Running || v.HostConfig == nil || !hasProfileMount(v.HostConfig, s.Directory()) || v.Config.Labels[proxyVaultLabel] != info.Fingerprint {
		return proxyAuthState{}, "", false, false
	}
	secret, _, err := proxyServerSecret()
	if err != nil || len(secret) < 32 {
		return proxyAuthState{}, "", false, false
	}
	adminURL := viper.GetString("proxy.admin_url")
	if adminURL == "" {
		adminURL = fmt.Sprintf("http://127.0.0.1:%d", proxyRunAdminPort)
	}
	p, err := authproxy.NewRemoteProxyContext(ctx, adminURL, fmt.Sprintf("http://%s:%d", proxyRunName, proxyRunPort), secret)
	if err != nil || p.AuthProfile() != name || p.ProfileConnectionVersion() == "" || v.Config.Labels[proxyProfileLabel] != profileProxyID(proxyRunArgs{profileName: name, profileDirectory: s.Directory(), profileConnection: p.ProfileConnectionVersion()}) {
		return proxyAuthState{}, "", false, false
	}
	return remoteProxyAuthState(p), p.ProfileConnectionVersion(), p.IsOpenAIMode(), true
}
