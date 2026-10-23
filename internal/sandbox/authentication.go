package sandbox

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/pacificbelt30/codex-dock/internal/workerpolicy"
)

const labelAuthenticationSession = labelPrefix + "authentication-session"

// AuthenticationHome exists only in a bounded, guest-owned tmpfs. The image
// root is read-only; neither the caller's HOME nor its workspace is mounted.
const AuthenticationHome = "/var/tmp/codex-home"

// RunAuthenticationContext boots an empty, attested Kata session for a fixed
// official CLI login. Login runs through streaming Exec rather than the normal
// image entrypoint/console, so credentials never enter persistent console logs.
// The caller must remove the session using a separate cleanup context on every
// exit path. The guest's main process additionally limits its lifetime.
func (m *Manager) RunAuthenticationContext(ctx context.Context, name, image, user string) (string, error) {
	if m.proxy != nil || m.httpProxyURL == "" {
		return "", fmt.Errorf("login requires a dedicated credential-free egress proxy")
	}
	if name == "" || image == "" || user == "" {
		return "", fmt.Errorf("login requires an explicit name, pinned image and non-root user")
	}
	if err := validateAuthenticationUser(user); err != nil {
		return "", err
	}
	inspected, err := m.cli.ImageInspect(ctx, image)
	if err != nil {
		return "", err
	}
	return m.RunContext(ctx, RunOptions{
		Name: name, Image: image, ContainerUser: user,
		Runtime: RuntimeKata, ResourceProfile: "small",
		ProjectDir: "/", NoGitMetadata: true, ReadOnlyRootFS: true,
		Detach: true, authenticationSession: true,
		Environment:      workerpolicy.Environment{ImageID: inspected.ID},
		Lifecycle:        workerpolicy.Lifecycle{MaxLifetime: 15 * time.Minute},
		TemporaryStorage: []workerpolicy.TemporaryStorage{{Target: AuthenticationHome, MaxBytes: 128 << 20}},
	})
}

func validateAuthenticationUser(user string) error {
	uid, gid, ok := strings.Cut(user, ":")
	if !ok {
		return fmt.Errorf("login requires a non-root numeric uid:gid")
	}
	for _, raw := range []string{uid, gid} {
		id, err := strconv.ParseUint(raw, 10, 32)
		if err != nil || id == 0 {
			return fmt.Errorf("login requires a non-root numeric uid:gid")
		}
	}
	return nil
}

func configureAuthenticationSession(c *container.Config, h *container.HostConfig, opts RunOptions, endpoint string) error {
	if err := validateAuthenticationUser(opts.ContainerUser); err != nil {
		return err
	}
	if !opts.ReadOnlyRootFS || opts.Runtime != RuntimeKata || len(h.Mounts) != 0 {
		return fmt.Errorf("login requires Kata, a read-only image and no host mounts")
	}
	proxy, err := kataProxyEndpoint(endpoint, opts.proxyHosts)
	if err != nil {
		return err
	}
	u, err := url.Parse(proxy)
	if err != nil || u.Scheme != "http" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
		return fmt.Errorf("invalid login egress endpoint")
	}
	c.Labels[labelAuthenticationSession] = "true"
	c.Entrypoint = []string{"/bin/sleep"}
	c.Cmd = []string{strconv.Itoa(int((15 * time.Minute).Seconds()))}
	c.WorkingDir = AuthenticationHome
	c.Env = []string{
		"HOME=" + AuthenticationHome,
		"XDG_CONFIG_HOME=" + AuthenticationHome + "/.config",
		"XDG_CACHE_HOME=" + AuthenticationHome + "/.cache",
		"CODEX_HOME=" + AuthenticationHome + "/.codex",
		"CLAUDE_CONFIG_DIR=" + AuthenticationHome + "/.claude",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1",
		"DISABLE_AUTOUPDATER=1", "BROWSER=/bin/true",
		"HTTP_PROXY=" + proxy, "HTTPS_PROXY=" + proxy,
		"http_proxy=" + proxy, "https_proxy=" + proxy,
		"NO_PROXY=localhost,127.0.0.1", "no_proxy=localhost,127.0.0.1",
	}
	return nil
}
