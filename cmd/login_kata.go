package cmd

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	dockernetwork "github.com/docker/docker/api/types/network"
	dockerdefaults "github.com/pacificbelt30/codex-dock/docker"
	"github.com/pacificbelt30/codex-dock/internal/authproxy"
	"github.com/pacificbelt30/codex-dock/internal/buildinfo"
	"github.com/pacificbelt30/codex-dock/internal/engine"
	"github.com/pacificbelt30/codex-dock/internal/kataguard"
	"github.com/pacificbelt30/codex-dock/internal/networkpolicy"
	"github.com/pacificbelt30/codex-dock/internal/sandbox"
	"github.com/pacificbelt30/codex-dock/pkg/cxd/credentialstore"
	"github.com/spf13/viper"
)

func ensureLoginImage(ctx context.Context, output io.Writer) (string, error) {
	recipe := append(append(append([]byte{}, dockerdefaults.Dockerfile...), dockerdefaults.Entrypoint...), buildinfo.Version...)
	digest := sha256.Sum256(recipe)
	tag := "oaiso-login:" + hex.EncodeToString(digest[:8])
	c, err := engine.NewClient()
	if err != nil {
		return "", err
	}
	defer func() { _ = c.Close() }()
	if _, err := c.ImageInspect(ctx, tag); err == nil {
		return tag, nil
	} else if !errdefs.IsNotFound(err) {
		return "", err
	}
	dir, err := os.MkdirTemp("", "oaiso-login-image-*")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	dockerfile := filepath.Join(dir, "Dockerfile")
	if err := os.WriteFile(dockerfile, dockerdefaults.Dockerfile, 0600); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "entrypoint.sh"), dockerdefaults.Entrypoint, 0600); err != nil {
		return "", err
	}
	if err := engine.BuildPinned(ctx, tag, dockerfile, dir, output); err != nil {
		return "", err
	}
	if _, err := c.ImageInspect(ctx, tag); err != nil {
		return "", err
	}
	return tag, nil
}

func subscriptionLogin(ctx context.Context, agent string, input io.Reader, output io.Writer) (credentials credentialstore.Credentials, result error) {
	if os.Geteuid() == 0 || os.Getegid() == 0 {
		return credentials, fmt.Errorf("run login as the normal operator")
	}
	if agent != "codex" && agent != "claude" {
		return credentials, fmt.Errorf("choose codex or claude")
	}
	var nonce [12]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return credentials, err
	}
	name := "oaiso-login-" + hex.EncodeToString(nonce[:])
	proxyName, networkName := name+"-egress", name+"-net"
	manager, err := sandbox.NewManager(sandbox.ManagerConfig{
		HTTPProxyURL: "http://" + proxyName + ":18082",
		KataConfig:   &kataguard.Config{Socket: viper.GetString("kata.node_socket")},
	})
	if err != nil {
		return credentials, err
	}
	defer func() { _ = manager.Close() }()
	if err := manager.CheckRuntime(ctx, sandbox.RunOptions{Runtime: sandbox.RuntimeKata, ResourceProfile: "small"}); err != nil {
		return credentials, err
	}
	image, err := ensureLoginImage(ctx, output)
	if err != nil {
		return credentials, err
	}
	proxyImage := viper.GetString("proxy_image")
	if proxyImage == "" {
		proxyImage = "codex-dock-proxy:latest"
	}
	if err := ensureProxyImage(ctx, proxyImage); err != nil {
		return credentials, err
	}
	c, err := engine.NewClient()
	if err != nil {
		return credentials, err
	}
	defer func() { _ = c.Close() }()
	var workerID string
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		if workerID != "" {
			if err := manager.RemoveContext(cleanup, workerID, true); err != nil {
				result = errors.Join(result, fmt.Errorf("login VM cleanup failed; recover worker %s: %w", workerID, err))
				return // Keep its egress attached until VM removal is confirmed.
			}
		}
		if err := c.ContainerRemove(cleanup, proxyName, container.RemoveOptions{Force: true}); err != nil && !errdefs.IsNotFound(err) {
			result = errors.Join(result, fmt.Errorf("login egress cleanup: %w", err))
			return
		}
		if n, err := c.NetworkInspect(cleanup, networkName, dockernetwork.InspectOptions{}); err == nil {
			result = errors.Join(result, c.NetworkRemove(cleanup, n.ID))
		} else if !errdefs.IsNotFound(err) {
			result = errors.Join(result, err)
		}
	}()
	if err := ensureBridgeNetwork(ctx, networkName, ""); err != nil {
		return credentials, err
	}
	domains := []string{"=auth.openai.com", "=chatgpt.com"}
	if agent == "claude" {
		domains = []string{"=claude.ai", "=console.anthropic.com", "=api.anthropic.com", "=platform.claude.com"}
	}
	if err := ensureProxyContainer(ctx, proxyRunArgs{
		name: proxyName, role: authproxy.RoleEgress, image: proxyImage,
		networkName: networkName, listenAddr: "0.0.0.0:18082", blockPrivate: true,
		forwardPolicy: networkpolicy.Rules{DefaultDeny: true, AllowDomains: domains, AllowPorts: []int{443}},
	}); err != nil {
		return credentials, err
	}
	workerID, err = manager.RunAuthenticationContext(ctx, name, image, fmt.Sprintf("%d:%d", os.Geteuid(), os.Getegid()))
	if err != nil {
		return credentials, err
	}
	loginCtx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	args := []string{"/usr/local/bin/codex", "-c", `cli_auth_credentials_store="file"`, "login", "--device-auth"}
	path, format := sandbox.AuthenticationHome+"/.codex/auth.json", "codex"
	if agent == "claude" {
		args = []string{"/usr/local/bin/claude", "auth", "login", "--claudeai"}
		path, format = sandbox.AuthenticationHome+"/.claude/.credentials.json", "claude"
	}
	code, err := manager.ExecWithOptions(loginCtx, workerID, sandbox.ExecOptions{
		Argv: args, WorkingDir: sandbox.AuthenticationHome, Stdin: input, Stdout: output, Stderr: output,
	})
	if err != nil || code != 0 {
		return credentials, fmt.Errorf("subscription login did not complete (exit %d); temporary credentials were not saved", code)
	}
	var buffer boundedCredentialBuffer
	code, err = manager.ExecWithOptions(loginCtx, workerID, sandbox.ExecOptions{
		Argv: []string{"/bin/cat", path}, WorkingDir: sandbox.AuthenticationHome, Stdout: &buffer,
	})
	defer clear(buffer.Bytes())
	if err != nil || code != 0 {
		return credentials, fmt.Errorf("could not collect login credentials; temporary credentials were not saved")
	}
	credentials, err = decodeProfileInput(buffer.Bytes(), format)
	if err != nil {
		return credentialstore.Credentials{}, fmt.Errorf("official CLI returned unsupported login credentials")
	}
	return credentials, nil
}

type boundedCredentialBuffer struct{ bytes.Buffer }

func (b *boundedCredentialBuffer) Write(data []byte) (int, error) {
	if b.Len()+len(data) > 1<<20 {
		return 0, fmt.Errorf("login credentials exceed 1 MiB")
	}
	return b.Buffer.Write(data)
}
