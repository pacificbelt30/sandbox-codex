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

func TestProxyContainerName(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
		ok   bool
	}{
		{name: "service name", raw: "http://codex-auth-proxy:18080", want: "codex-auth-proxy", ok: true},
		{name: "host-gateway alias", raw: "http://host.docker.internal:18080", ok: false},
		{name: "ip literal", raw: "http://10.200.0.1:18080", ok: false},
		{name: "bad", raw: "://bad", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := proxyContainerName(tt.raw)
			if ok != tt.ok {
				t.Fatalf("proxyContainerName(%q) ok=%v want=%v", tt.raw, ok, tt.ok)
			}
			if got != tt.want {
				t.Fatalf("proxyContainerName(%q)=%q want=%q", tt.raw, got, tt.want)
			}
		})
	}
}
