package cmd

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// TestResolveContainerUser_Empty verifies that an empty mode returns "" (image default).
func TestResolveContainerUser_Empty(t *testing.T) {
	got, err := resolveContainerUser("", "/tmp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("got %q; want \"\"", got)
	}
}

// TestResolveContainerUser_Current verifies that "current" returns the running
// process's uid:gid.
func TestResolveContainerUser_Current(t *testing.T) {
	uid := syscall.Getuid()
	gid := syscall.Getgid()
	want := fmt.Sprintf("%d:%d", uid, gid)

	got, err := resolveContainerUser("current", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("got %q; want %q", got, want)
	}
}

func TestResolveContainerUser_Codex(t *testing.T) {
	got, err := resolveContainerUser("codex", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "1001:1001" {
		t.Errorf("got %q; want %q", got, "1001:1001")
	}
}

// TestResolveContainerUser_Dir verifies that "dir" returns the uid:gid of the
// directory owner.
func TestResolveContainerUser_Dir(t *testing.T) {
	dir := t.TempDir()

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat tempdir: %v", err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Skip("cannot cast to *syscall.Stat_t on this platform")
	}
	want := fmt.Sprintf("%d:%d", stat.Uid, stat.Gid)

	got, err := resolveContainerUser("dir", dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("got %q; want %q", got, want)
	}
}

// TestResolveContainerUser_Dir_NotExist verifies that "dir" with a missing path
// returns an error.
func TestResolveContainerUser_Dir_NotExist(t *testing.T) {
	_, err := resolveContainerUser("dir", "/nonexistent-path-codex-dock-test")
	if err == nil {
		t.Error("expected error for non-existent directory, got nil")
	}
}

// TestResolveContainerUser_Explicit verifies that an explicit uid or uid:gid is
// passed through unchanged.
func TestResolveContainerUser_Explicit(t *testing.T) {
	cases := []string{"1000", "1000:1000", "500:500", "0"}
	for _, tc := range cases {
		got, err := resolveContainerUser(tc, "")
		if err != nil {
			t.Errorf("resolveContainerUser(%q): unexpected error: %v", tc, err)
			continue
		}
		if got != tc {
			t.Errorf("resolveContainerUser(%q) = %q; want %q", tc, got, tc)
		}
	}
}

// TestResolveContainerUser_Current_Format verifies the output has "uid:gid" shape.
func TestResolveContainerUser_Current_Format(t *testing.T) {
	got, err := resolveContainerUser("current", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	parts := strings.Split(got, ":")
	if len(parts) != 2 {
		t.Errorf("expected uid:gid format, got %q", got)
	}
	for _, p := range parts {
		if p == "" {
			t.Errorf("empty part in uid:gid %q", got)
		}
	}
}

// TestResolveProjectDir_Dot verifies that "." resolves to the working directory.
func TestResolveProjectDir_Dot(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	got, err := resolveProjectDir(".")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != wd {
		t.Errorf("got %q; want %q", got, wd)
	}
}

// TestResolveProjectDir_Absolute verifies that an absolute path is returned as-is.
func TestResolveProjectDir_Absolute(t *testing.T) {
	got, err := resolveProjectDir("/tmp/myproject")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/tmp/myproject" {
		t.Errorf("got %q; want /tmp/myproject", got)
	}
}

func TestApplyRunConfigDefaults(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	prevRunOpts := runOpts
	prevUserMode := userMode
	prevApprovalMode := approvalModeFlag
	t.Cleanup(func() {
		runOpts = prevRunOpts
		userMode = prevUserMode
		approvalModeFlag = prevApprovalMode
	})

	runOpts.Image = "codex-dock:latest"
	runOpts.TokenTTL = 3600
	userMode = "current"
	approvalModeFlag = "suggest"

	viper.Set("run.image", "custom-image:1")
	viper.Set("run.token_ttl", 90)
	viper.Set("run.user", "dir")
	viper.Set("run.approval_mode", "auto-edit")

	cmd := &cobra.Command{Use: "run"}
	cmd.Flags().String("image", "", "")
	cmd.Flags().Int("token-ttl", 0, "")
	cmd.Flags().String("approval-mode", "", "")
	cmd.Flags().String("user", "", "")

	applyRunConfigDefaults(cmd)

	if runOpts.Image != "custom-image:1" {
		t.Errorf("runOpts.Image = %q; want custom-image:1", runOpts.Image)
	}
	if runOpts.TokenTTL != 90 {
		t.Errorf("runOpts.TokenTTL = %d; want 90", runOpts.TokenTTL)
	}
	if userMode != "dir" {
		t.Errorf("userMode = %q; want dir", userMode)
	}
	if approvalModeFlag != "auto-edit" {
		t.Errorf("approvalModeFlag = %q; want auto-edit", approvalModeFlag)
	}
}

func TestIsProxyUnreachable(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{fmt.Errorf("dial tcp 127.0.0.1:18081: connect: connection refused"), true},
		{fmt.Errorf("lookup codex-auth-proxy: no such host"), true},
		{fmt.Errorf("dial tcp: i/o timeout"), true},
		{fmt.Errorf("checking remote auth proxy mode failed: status 401"), false},
		{fmt.Errorf("invalid json"), false},
	}
	for _, c := range cases {
		if got := isProxyUnreachable(c.err); got != c.want {
			t.Errorf("isProxyUnreachable(%v) = %v; want %v", c.err, got, c.want)
		}
	}
}

func TestConfirmYesNo(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"y\n", true},
		{"Y\n", true},
		{"yes\n", true},
		{"  yes  \n", true},
		{"n\n", false},
		{"\n", false}, // bare Enter defaults to no
		{"nope\n", false},
	}
	for _, c := range cases {
		if got := confirmYesNo(strings.NewReader(c.in), "? "); got != c.want {
			t.Errorf("confirmYesNo(%q) = %v; want %v", c.in, got, c.want)
		}
	}
}

// TestRunKeepFlag asserts the --keep opt-out for foreground cleanup exists and
// defaults to removing the container/network (keep = false).
func TestRunKeepFlag(t *testing.T) {
	f := runCmd.Flags().Lookup("keep")
	if f == nil {
		t.Fatal("run: --keep flag not registered")
	}
	if f.DefValue != "false" {
		t.Errorf("--keep default = %q; want false (clean up by default)", f.DefValue)
	}
}

// TestRunNoFirewallFlagRemoved asserts the iptables-era flags are gone now that
// isolation is enforced entirely by Docker networks.
func TestRunNoFirewallFlagRemoved(t *testing.T) {
	for _, name := range []string{"no-firewall", "sudo", "allow-host", "block-host"} {
		if f := runCmd.Flags().Lookup(name); f != nil {
			t.Errorf("run: --%s flag should have been removed", name)
		}
	}
}

func TestApplyRunConfigDefaults_ProxyURL(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	prevURL := proxyContainerURL
	t.Cleanup(func() { proxyContainerURL = prevURL })

	// Legacy [firewall] section is still honored for the proxy URL.
	proxyContainerURL = "http://codex-auth-proxy:18080"
	viper.Set("firewall.proxy_container_url", "http://proxy.internal:9000")

	cmd := &cobra.Command{Use: "run"}
	cmd.Flags().String("image", "", "")
	cmd.Flags().Int("token-ttl", 0, "")
	cmd.Flags().String("approval-mode", "", "")
	cmd.Flags().String("user", "", "")
	cmd.Flags().String("proxy-container-url", "http://codex-auth-proxy:18080", "")

	applyRunConfigDefaults(cmd)

	if proxyContainerURL != "http://proxy.internal:9000" {
		t.Errorf("proxyContainerURL = %q; want legacy config value", proxyContainerURL)
	}

	// The new [proxy] section takes precedence.
	viper.Set("proxy.container_url", "http://codex-auth-proxy:18080")
	applyRunConfigDefaults(cmd)
	if proxyContainerURL != "http://codex-auth-proxy:18080" {
		t.Errorf("proxyContainerURL = %q; want [proxy] section value", proxyContainerURL)
	}
}

func TestApplyRunConfigDefaults_FlagPriority(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	prevRunOpts := runOpts
	prevUserMode := userMode
	prevApprovalMode := approvalModeFlag
	t.Cleanup(func() {
		runOpts = prevRunOpts
		userMode = prevUserMode
		approvalModeFlag = prevApprovalMode
	})

	runOpts.Image = "flag-image"
	runOpts.TokenTTL = 123
	userMode = "current"
	approvalModeFlag = "danger"

	viper.Set("run.image", "config-image")
	viper.Set("run.token_ttl", 999)
	viper.Set("run.user", "dir")
	viper.Set("run.approval_mode", "auto-edit")

	cmd := &cobra.Command{Use: "run"}
	cmd.Flags().String("image", "", "")
	cmd.Flags().Int("token-ttl", 0, "")
	cmd.Flags().String("approval-mode", "", "")
	cmd.Flags().String("user", "", "")

	if err := cmd.Flags().Set("image", "flag-image"); err != nil {
		t.Fatalf("set image: %v", err)
	}
	if err := cmd.Flags().Set("token-ttl", "123"); err != nil {
		t.Fatalf("set token-ttl: %v", err)
	}
	if err := cmd.Flags().Set("approval-mode", "danger"); err != nil {
		t.Fatalf("set approval-mode: %v", err)
	}
	if err := cmd.Flags().Set("user", "current"); err != nil {
		t.Fatalf("set user: %v", err)
	}

	applyRunConfigDefaults(cmd)

	if runOpts.Image != "flag-image" {
		t.Errorf("runOpts.Image = %q; want flag-image", runOpts.Image)
	}
	if runOpts.TokenTTL != 123 {
		t.Errorf("runOpts.TokenTTL = %d; want 123", runOpts.TokenTTL)
	}
	if userMode != "current" {
		t.Errorf("userMode = %q; want current", userMode)
	}
	if approvalModeFlag != "danger" {
		t.Errorf("approvalModeFlag = %q; want danger", approvalModeFlag)
	}
}
