package sandbox

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
)

func TestAuthenticationSessionDoesNotInheritWorkspaceOrEnvironment(t *testing.T) {
	opts := RunOptions{authenticationSession: true, Runtime: RuntimeKata, ReadOnlyRootFS: true, ContainerUser: "1000:1000", Mounts: []MountSpec{{Source: "/private", Target: "/shared"}}}
	if got := buildMounts(opts, "/private/project"); len(got) != 0 {
		t.Fatal("authentication session inherited host shares")
	}
	c := &container.Config{Labels: map[string]string{}, Env: []string{"OPENAI_API_KEY=private", "CLAUDE_CODE_OAUTH_TOKEN=private", "DOCK_ENVIRONMENT_JSON=private"}}
	h := &container.HostConfig{ReadonlyRootfs: true}
	opts.proxyHosts = []string{"login-egress:10.253.4.2"}
	if err := configureAuthenticationSession(c, h, opts, "http://login-egress:18082"); err != nil {
		t.Fatal(err)
	}
	if c.Labels[labelAuthenticationSession] != "true" || !slices.Equal(c.Entrypoint, []string{"/bin/sleep"}) || !slices.Equal(c.Cmd, []string{"900"}) || c.WorkingDir != AuthenticationHome {
		t.Fatal("login did not bypass the image entrypoint or bound its lifetime")
	}
	env := strings.Join(c.Env, "\n")
	if strings.Contains(env, "private") || strings.Contains(env, "DOCK_") || !strings.Contains(env, "HTTPS_PROXY=http://10.253.4.2:18082") {
		t.Fatal("login inherited credentials/hooks or failed to pin its proxy address")
	}
}

func TestAuthenticationSessionRejectsUnsafeConfiguration(t *testing.T) {
	valid := RunOptions{Runtime: RuntimeKata, ReadOnlyRootFS: true, ContainerUser: "1000:1000"}
	for _, user := range []string{"0:0", "0:1000", "1000:0", "user", "1000", "-1:1000", "1000:1000:1"} {
		opts := valid
		opts.ContainerUser = user
		if err := configureAuthenticationSession(&container.Config{Labels: map[string]string{}}, &container.HostConfig{}, opts, "http://egress:18082"); err == nil {
			t.Fatalf("accepted unsafe identity %q", user)
		}
	}
	for _, endpoint := range []string{"http://key:secret@egress:18082", "http://egress:18082/?token=secret", "http://egress:18082/#secret", "https://egress:18082", "http://egress:18082/path"} {
		if err := configureAuthenticationSession(&container.Config{Labels: map[string]string{}}, &container.HostConfig{}, valid, endpoint); err == nil {
			t.Fatal("accepted unsafe proxy endpoint")
		}
	}
	if err := configureAuthenticationSession(&container.Config{Labels: map[string]string{}}, &container.HostConfig{Mounts: []mount.Mount{{Source: "/private", Target: "/workspace"}}}, valid, "http://egress:18082"); err == nil {
		t.Fatal("accepted a host mount")
	}
	for _, runtime := range []string{"", RuntimeRunc} {
		opts := valid
		opts.Runtime = runtime
		if err := configureAuthenticationSession(&container.Config{Labels: map[string]string{}}, &container.HostConfig{}, opts, "http://egress:18082"); err == nil {
			t.Fatal("accepted a non-Kata authentication session")
		}
	}
	if _, err := (&Manager{}).RunAuthenticationContext(context.Background(), "login", "image", "1000:1000"); err == nil {
		t.Fatal("authentication session accepted a missing egress proxy")
	}
}
