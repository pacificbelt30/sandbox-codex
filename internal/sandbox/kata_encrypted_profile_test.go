package sandbox_test

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	dn "github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
	"github.com/pacificbelt30/codex-dock/internal/authproxy"
	"github.com/pacificbelt30/codex-dock/internal/engine"
	"github.com/pacificbelt30/codex-dock/pkg/cxd"
	"github.com/pacificbelt30/codex-dock/pkg/cxd/credentialstore"
)

//go:embed testdata/legacy-encrypted-profile.json
var legacyEncryptedProfileFixture []byte

// Exercises ciphertext mounts, secret stdin IPC and a real Kata worker. All
// credentials are deliberately invalid; no provider request is made.
func TestKataEncryptedProfile(t *testing.T) {
	if os.Getenv("CODEX_DOCK_KATA_E2E") != "1" {
		t.Skip("requires a disposable Linux/KVM node")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	node, err := engine.NewClient()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = node.Close() })
	verifyLegacyAgeMigration(t, ctx)
	prefix := fmt.Sprintf("encrypted-kata-%d", time.Now().UnixNano())
	bridge, err := node.NetworkCreate(ctx, prefix+"-net", dn.CreateOptions{Driver: "bridge"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := node.NetworkRemove(context.Background(), bridge.ID); err != nil {
			t.Error(err)
		}
	})
	profileRoot := t.TempDir()
	if err = os.Chmod(profileRoot, 0700); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(profileRoot, "work")
	store, err := credentialstore.NewEncryptedStore(dir, "work")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	secret := "fixture-encrypted-api-never-valid"
	pass := []byte("fixture long encrypted passphrase")
	if err = store.CreateWithPassphrase(ctx, "work", pass, credentialstore.Credentials{OpenAIAPIKey: secret}); err != nil {
		t.Fatal(err)
	}
	key, err := store.UnlockedKey()
	if err != nil {
		t.Fatal(err)
	}
	defer clear(key)
	var keyFields struct {
		Identity          string `json:"identity"`
		AuthenticationKey []byte `json:"authentication_key"`
	}
	if err = json.Unmarshal(key, &keyFields); err != nil || keyFields.Identity == "" {
		t.Fatal("age identity bundle is invalid")
	}
	defer clear(keyFields.AuthenticationKey)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	admin := "fixture-private-admin-authority-32-bytes"
	authority := t.TempDir() + "/admin"
	if err = os.WriteFile(authority, []byte(admin), 0600); err != nil {
		t.Fatal(err)
	}
	pport := nat.Port(fmt.Sprintf("%d/tcp", port))
	cfg := &container.Config{Image: os.Getenv("CODEX_DOCK_KATA_PROXY_IMAGE"), User: fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()), OpenStdin: true, Labels: map[string]string{"io.codex-dock.proxy-layout": "encrypted-acceptance"}, Cmd: []string{"proxy", "serve", "--auth-profile", "work", "--profile-store-dir", "/run/cxd-profile", "--profile-key-stdin", "--role", "auth", "--listen", "0.0.0.0:18080", "--admin-listen", fmt.Sprintf("egress:%d", port), "--admin-secret-file", "/run/cxd-admin-secret", "--block-private"}}
	host := &container.HostConfig{Runtime: "runc", NetworkMode: container.NetworkMode(bridge.ID), CapDrop: []string{"ALL"}, SecurityOpt: []string{"no-new-privileges:true"}, Mounts: []mount.Mount{{Type: mount.TypeBind, Source: dir, Target: "/run/cxd-profile"}, {Type: mount.TypeBind, Source: authority, Target: "/run/cxd-admin-secret", ReadOnly: true}}, PortBindings: nat.PortMap{pport: []nat.PortBinding{{HostIP: "127.0.0.1", HostPort: fmt.Sprint(port)}}}}
	created, err := node.ContainerCreate(ctx, cfg, host, nil, nil, prefix)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := node.ContainerRemove(context.Background(), created.ID, container.RemoveOptions{Force: true}); err != nil && !errdefs.IsNotFound(err) {
			t.Error(err)
		}
	})
	sendKey := func() {
		attached, err := node.ContainerAttach(ctx, created.ID, container.AttachOptions{Stream: true, Stdin: true})
		if err != nil {
			logs, logErr := node.ContainerLogs(ctx, created.ID, container.LogsOptions{ShowStdout: true, ShowStderr: true})
			if logErr == nil {
				data, _ := io.ReadAll(io.LimitReader(logs, 8192))
				_ = logs.Close()
				for _, v := range [][]byte{[]byte(secret), pass, key} {
					data = bytes.ReplaceAll(data, v, []byte("[redacted]"))
				}
				t.Logf("proxy startup diagnostics: %s", data)
			}
			t.Fatal(err)
		}
		_ = attached.Conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err = credentialstore.WriteUnlockKey(attached.Conn, key); err != nil {
			t.Fatal(err)
		}
		_ = attached.CloseWrite()
		attached.Close()
	}
	if err = node.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		t.Fatal(err)
	}
	sendKey()
	adminURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	containerURL := "http://" + prefix + ":18080"
	var proxy *authproxy.RemoteProxy
	for i := 0; i < 40; i++ {
		proxy, err = authproxy.NewRemoteProxyContext(ctx, adminURL, containerURL, admin)
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	saved, _ := store.Load(ctx, "work")
	if proxy.AuthProfile() != "work" || proxy.ProfileConnectionVersion() != saved.ConnectionVersion || !proxy.IsOpenAIMode() {
		t.Fatal("proxy loaded incorrect encrypted profile")
	}
	api, err := cxd.Open(ctx, cxd.Options{ProxyAdminURL: adminURL, ProxyContainerURL: containerURL, ProxyAdminSecret: admin, StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = api.Close() })
	worker, err := api.Launch(ctx, cxd.LaunchRequest{RequestID: prefix, Image: os.Getenv("CODEX_DOCK_KATA_TEST_IMAGE"), ProjectDir: t.TempDir(), Profile: "small", NoInternet: true, User: fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid())})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := api.Remove(context.Background(), worker.ID, true); err != nil {
			t.Error(err)
		}
	})
	inspected, err := node.ContainerInspect(ctx, worker.ID)
	if err != nil {
		t.Fatal(err)
	}
	metadata, _ := json.Marshal(inspected)
	for _, private := range []string{secret, string(pass), string(key), keyFields.Identity, base64.StdEncoding.EncodeToString(keyFields.AuthenticationKey), dir, authority} {
		if bytes.Contains(metadata, []byte(private)) {
			t.Fatal("worker metadata contains a secret or private host share")
		}
	}
	var output bytes.Buffer
	code, err := api.Exec(ctx, worker.ID, cxd.ExecRequest{Argv: []string{"python3", "-c", `import os,pathlib
assert not pathlib.Path('/run/cxd-profile').exists()
assert not pathlib.Path('/run/cxd-admin-secret').exists()
h=pathlib.Path.home()
for p in h.rglob('*'):
 if p.is_file():
  try:
   assert b'fixture-encrypted-api-never-valid' not in p.read_bytes()
   assert b'AGE-SECRET-KEY-1' not in p.read_bytes()
  except PermissionError: pass
assert 'fixture-encrypted-api-never-valid' not in str(dict(os.environ))
assert 'AGE-SECRET-KEY-1' not in str(dict(os.environ))
print('encrypted profile remains host-only')`}, Stdout: &output, Stderr: &output})
	if err != nil || code != 0 {
		t.Fatalf("guest secret check failed: %v %d %s", err, code, output.String())
	}
	logs, err := node.ContainerLogs(ctx, created.ID, container.LogsOptions{ShowStdout: true, ShowStderr: true})
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(logs)
	_ = logs.Close()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, key) || bytes.Contains(data, []byte(secret)) || bytes.Contains(data, pass) || bytes.Contains(data, []byte(keyFields.Identity)) || bytes.Contains(data, keyFields.AuthenticationKey) {
		t.Fatal("proxy console leaked credentials or input key")
	}
	// Removing the proxy discards its in-memory token table. Ciphertext alone is
	// insufficient to restart it and worker connections cannot bypass it.
	config := t.TempDir() + "/config.toml"
	if err = os.WriteFile(config, []byte(fmt.Sprintf("[proxy]\nadmin_secret_file = %q\nadmin_url = %q\ncontainer_url = %q\n", authority, adminURL, containerURL)), 0600); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(ctx, os.Getenv("CODEX_DOCK_KATA_TEST_BINARY"), "--config", config, "--auth-profile-dir", profileRoot, "auth", "profile", "lock", "work")
	if result, err := command.CombinedOutput(); err != nil {
		t.Fatalf("profile lock command failed: %v %s", err, result)
	}
	for _, id := range []string{worker.ID, created.ID} {
		v, err := node.ContainerInspect(ctx, id)
		if err != nil || v.State == nil || v.State.Running {
			t.Fatal("profile lock left a worker/proxy running", err)
		}
	}
	if err = node.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if _, err = authproxy.NewRemoteProxyContext(ctx, adminURL, containerURL, admin); err == nil {
		t.Fatal("encrypted proxy started without key input")
	}
	sendKey()
	for i := 0; i < 40; i++ {
		_, err = authproxy.NewRemoteProxyContext(ctx, adminURL, containerURL, admin)
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Log("verified: ciphertext-only proxy mount, secret stdin IPC, real Kata guest without credentials, secret-free logs, locked restart and explicit unlock")

	// Exercise the production CLI's unlock/transfer/reuse path as well as the
	// lower-level API fixture above. The second call has no passphrase input.
	listener, err = net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	cliPort := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	cliName, egressName, netName := prefix+"-cli", prefix+"-cli-egress", prefix+"-cli-net"
	t.Cleanup(func() {
		for _, name := range []string{cliName, egressName} {
			if err := node.ContainerRemove(context.Background(), name, container.RemoveOptions{Force: true}); err != nil && !errdefs.IsNotFound(err) {
				t.Error(err)
			}
		}
		if err := node.NetworkRemove(context.Background(), netName); err != nil && !errdefs.IsNotFound(err) {
			t.Error(err)
		}
	})
	if err = os.WriteFile(config, []byte(fmt.Sprintf("proxy_image = %q\n[proxy]\nadmin_secret_file = %q\nadmin_url = %q\n", os.Getenv("CODEX_DOCK_KATA_PROXY_IMAGE"), authority, fmt.Sprintf("http://127.0.0.1:%d", cliPort))), 0600); err != nil {
		t.Fatal(err)
	}
	cliArgs := []string{"--config", config, "--auth-profile-dir", profileRoot, "--auth-profile", "work", "proxy", "run", "--name", cliName, "--admin-port", fmt.Sprint(cliPort), "--http-name", egressName, "--network", netName}
	for _, unlock := range []bool{true, false} {
		args := append([]string{}, cliArgs...)
		var input *os.File
		if unlock {
			args = append([]string{"--passphrase-fd", "3"}, args...)
			var writer *os.File
			input, writer, err = os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			_, _ = writer.Write(pass)
			_ = writer.Close()
		}
		command := exec.CommandContext(ctx, os.Getenv("CODEX_DOCK_KATA_TEST_BINARY"), args...)
		if input != nil {
			command.ExtraFiles = []*os.File{input}
		}
		result, runErr := command.CombinedOutput()
		if input != nil {
			_ = input.Close()
		}
		if runErr != nil {
			t.Fatalf("CLI encrypted proxy unlock/reuse failed: %v %s", runErr, result)
		}
		if bytes.Contains(result, pass) || bytes.Contains(result, key) || bytes.Contains(result, []byte(secret)) {
			t.Fatal("CLI leaked an encryption secret")
		}
	}
	t.Log("verified: production CLI unlocks through secret stdin and reuses the authenticated proxy without another passphrase")

}

func verifyLegacyAgeMigration(t *testing.T, ctx context.Context) {
	t.Helper()
	var fixture struct {
		Passphrase string
		Vault      json.RawMessage
	}
	if err := json.Unmarshal(legacyEncryptedProfileFixture, &fixture); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := credentialstore.NewEncryptedStore(filepath.Join(root, "work"), "work")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	path := filepath.Join(store.Directory(), "work.vault")
	if err = os.WriteFile(path, fixture.Vault, 0600); err != nil {
		t.Fatal(err)
	}
	if err = store.Unlock(ctx, []byte(fixture.Passphrase)); err != nil {
		t.Fatal(err)
	}
	before, err := store.Load(ctx, "work")
	if err != nil {
		t.Fatal(err)
	}
	input, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = input.Close() }()
	_, _ = writer.Write([]byte(fixture.Passphrase))
	_ = writer.Close()
	command := exec.CommandContext(ctx, os.Getenv("CODEX_DOCK_KATA_TEST_BINARY"), "--passphrase-fd", "3", "--auth-profile-dir", root, "auth", "profile", "migrate-age", "work")
	command.ExtraFiles = []*os.File{input}
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("legacy migration CLI failed: %v %s", err, output)
	}
	if bytes.Contains(output, []byte(fixture.Passphrase)) || bytes.Contains(output, []byte(before.OpenAIOAuth.RefreshToken)) {
		t.Fatal("migration CLI exposed credentials")
	}
	info, err := store.Info(ctx)
	if err != nil || info.Format != "age" {
		t.Fatal("migration CLI did not create age storage", err)
	}
	_ = store.Close()
	if err = store.Unlock(ctx, []byte(fixture.Passphrase)); err != nil {
		t.Fatal(err)
	}
	after, err := store.Load(ctx, "work")
	if err != nil || after.ConnectionVersion == before.ConnectionVersion || after.OpenAIOAuth.RefreshToken != before.OpenAIOAuth.RefreshToken {
		t.Fatal("migration did not preserve credentials and replace the connection", err)
	}
	backup, err := os.Stat(path + ".v1-backup")
	if err != nil || backup.Mode().Perm() != 0600 {
		t.Fatal("migration recovery backup is missing")
	}
	t.Log("verified: production CLI migrates original schema-1 ciphertext to age, rotates connection generation and retains an encrypted 0600 recovery backup")
}
