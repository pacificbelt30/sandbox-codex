package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/pacificbelt30/codex-dock/internal/sandbox"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func TestRunDetachUsesDockerShorthand(t *testing.T) {
	if got := runCmd.Flags().Lookup("detach").Shorthand; got != "d" {
		t.Fatalf("--detach shorthand = %q; want -d", got)
	}
	if got := runCmd.Flags().Lookup("project").Shorthand; got != "" {
		t.Fatalf("--project shorthand = %q; want long flag only", got)
	}
	if got := runCmd.Flags().Lookup("detach-legacy").Shorthand; got != "D" {
		t.Fatalf("legacy detach shorthand = %q; want -D", got)
	}
}

func TestExplicitTemplateIgnoresProjectBuildSource(t *testing.T) {
	cfg := &ProjectConfig{}
	cfg.Build.Dockerfile = "Dockerfile.codex-dock"
	cfg.Build.Tag = "codex-dock:latest"
	cmd := &cobra.Command{Use: "run"}
	cmd.Flags().String("template", "", "")
	if got := projectBuildConfigForRun(cmd, cfg); got != cfg {
		t.Fatal("project build source must apply without an explicit template")
	}
	if err := cmd.Flags().Set("template", "plain"); err != nil {
		t.Fatal(err)
	}
	if got := projectBuildConfigForRun(cmd, cfg); got != nil {
		t.Fatal("explicit template must select its own build source")
	}
}

func TestRunDockerPositionals(t *testing.T) {
	for _, tc := range []struct {
		name, image string
		argv        []string
		command     []string
	}{
		{"image and command flags", "codex-dock:latest", []string{"-d", "codex-dock:latest", "npm", "test", "--watch=false"}, []string{"npm", "test", "--watch=false"}},
		{"image with separator", "codex-dock:latest", []string{"codex-dock:latest", "--", "sh", "-c", "echo ok"}, []string{"sh", "-c", "echo ok"}},
		{"default image command", "", []string{"--", "ls", "-la"}, []string{"ls", "-la"}},
		{"explicit image flag", "", []string{"--image", "custom:latest", "--", "ls", "-la"}, []string{"ls", "-la"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "run"}
			cmd.Flags().SetInterspersed(false)
			cmd.Flags().BoolP("detach", "d", false, "")
			cmd.Flags().String("image", "", "")
			cmd.SetArgs(tc.argv)
			cmd.RunE = func(cmd *cobra.Command, args []string) error {
				image, command := resolveRunPositionals(args, cmd.ArgsLenAtDash(), cmd.Flags().Changed("image"))
				if image != tc.image || !slices.Equal(command, tc.command) {
					t.Errorf("image=%q command=%q; want image=%q command=%q", image, command, tc.image, tc.command)
				}
				return nil
			}
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

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

// TestParseMountSpecs_Empty verifies nil input yields no mounts.
func TestParseMountSpecs_Empty(t *testing.T) {
	got, err := parseMountSpecs(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("got %+v; want nil", got)
	}
}

// TestParseMountSpecs_ReadWrite verifies the default mode (no suffix or ":rw") is read-write.
func TestParseMountSpecs_ReadWrite(t *testing.T) {
	for _, spec := range []string{"/host/data:/data", "/host/data:/data:rw"} {
		got, err := parseMountSpecs([]string{spec})
		if err != nil {
			t.Fatalf("parseMountSpecs(%q): unexpected error: %v", spec, err)
		}
		if len(got) != 1 || got[0].Source != "/host/data" || got[0].Target != "/data" || got[0].ReadOnly {
			t.Errorf("parseMountSpecs(%q) = %+v", spec, got)
		}
	}
}

// TestParseMountSpecs_ReadOnly verifies the ":ro" suffix sets ReadOnly.
func TestParseMountSpecs_ReadOnly(t *testing.T) {
	got, err := parseMountSpecs([]string{"/host/data:/data:ro"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || !got[0].ReadOnly {
		t.Errorf("got %+v; want ReadOnly=true", got)
	}
}

// TestParseMountSpecs_RelativeHostPath verifies a relative host path is resolved to absolute.
func TestParseMountSpecs_RelativeHostPath(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	got, err := parseMountSpecs([]string{"relative/dir:/data"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := wd + "/relative/dir"
	if len(got) != 1 || got[0].Source != want {
		t.Errorf("got %+v; want Source %q", got, want)
	}
}

// TestParseMountSpecs_Multiple verifies multiple --mount flags are all parsed.
func TestParseMountSpecs_Multiple(t *testing.T) {
	got, err := parseMountSpecs([]string{"/host/a:/a:ro", "/host/b:/b"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d; want 2", len(got))
	}
}

// TestParseMountSpecs_Invalid covers rejected --mount values.
func TestParseMountSpecs_Invalid(t *testing.T) {
	tests := []string{
		"no-colon",
		"/host:/a:ro:extra",
		":/data",
		"/host:",
		"/host:relative/path",
		"/host:/workspace",
		"/host:/data:bogus",
	}
	for _, spec := range tests {
		if _, err := parseMountSpecs([]string{spec}); err == nil {
			t.Errorf("parseMountSpecs(%q): expected error, got nil", spec)
		}
	}
}

// TestParseMountSpecs_DuplicateTarget verifies mounting the same container path twice is rejected.
func TestParseMountSpecs_DuplicateTarget(t *testing.T) {
	if _, err := parseMountSpecs([]string{"/host/a:/data", "/host/b:/data"}); err == nil {
		t.Error("expected error for duplicate container path, got nil")
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

// TestApplyRunConfigDefaults_Mounts verifies that run.mounts in the config
// file populates mountFlags when --mount was not passed on the CLI, and is
// ignored when --mount was explicitly set.
func TestApplyRunConfigDefaults_Mounts(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	prevMountFlags := mountFlags
	t.Cleanup(func() { mountFlags = prevMountFlags })

	viper.Set("run.mounts", []string{"/host/data:/data:ro", "./cache:/cache"})

	cmd := &cobra.Command{Use: "run"}
	cmd.Flags().StringArray("mount", nil, "")

	mountFlags = nil
	applyRunConfigDefaults(cmd)

	want := []string{"/host/data:/data:ro", "./cache:/cache"}
	if len(mountFlags) != len(want) {
		t.Fatalf("mountFlags = %+v; want %+v", mountFlags, want)
	}
	for i := range want {
		if mountFlags[i] != want[i] {
			t.Errorf("mountFlags[%d] = %q; want %q", i, mountFlags[i], want[i])
		}
	}

	// Explicit --mount on the CLI takes precedence over the config file.
	if err := cmd.Flags().Set("mount", "/explicit:/explicit"); err != nil {
		t.Fatalf("setting --mount: %v", err)
	}
	mountFlags = []string{"/explicit:/explicit"}
	applyRunConfigDefaults(cmd)
	if len(mountFlags) != 1 || mountFlags[0] != "/explicit:/explicit" {
		t.Errorf("mountFlags = %+v; want CLI value preserved", mountFlags)
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

func TestRunForceRecreateProxyFlag(t *testing.T) {
	f := runCmd.Flags().Lookup("force-recreate-proxy")
	if f == nil {
		t.Fatal("run command is missing --force-recreate-proxy")
	}
	if f.DefValue != "false" {
		t.Fatalf("--force-recreate-proxy default = %q; want false", f.DefValue)
	}
}

func TestProxyAuthStateMismatch(t *testing.T) {
	oauth := proxyAuthState{openAIOAuth: true, anthropicAvailable: true, anthropicOAuth: true}
	if proxyAuthStateMismatch(oauth, oauth) {
		t.Fatal("equal auth states must not trigger proxy recreation")
	}

	cases := []proxyAuthState{
		{openAIOAuth: false, anthropicAvailable: true, anthropicOAuth: true},
		{openAIOAuth: true, anthropicAvailable: false, anthropicOAuth: false},
		{openAIOAuth: true, anthropicAvailable: true, anthropicOAuth: false},
	}
	for _, remote := range cases {
		if !proxyAuthStateMismatch(oauth, remote) {
			t.Fatalf("different auth states must trigger recreation: host=%+v remote=%+v", oauth, remote)
		}
	}
}

func TestRunningWorkersBlockProxyRecreation(t *testing.T) {
	if runningWorkersBlockProxyRecreation(0, false) {
		t.Fatal("zero workers must allow automatic proxy recreation")
	}
	if !runningWorkersBlockProxyRecreation(1, false) {
		t.Fatal("running workers must block automatic proxy recreation")
	}
	if runningWorkersBlockProxyRecreation(1, true) {
		t.Fatal("--force-recreate-proxy must allow recreation with running workers")
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

func TestApplyRunConfigDefaults_Template(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	prevRunOpts := runOpts
	t.Cleanup(func() { runOpts = prevRunOpts })

	runOpts.Image = "codex-dock:latest"

	viper.Set("default_template", "pwn")

	cmd := &cobra.Command{Use: "run"}
	cmd.Flags().String("image", "", "")
	cmd.Flags().Int("token-ttl", 0, "")
	cmd.Flags().String("approval-mode", "", "")
	cmd.Flags().String("user", "", "")
	cmd.Flags().String("proxy-container-url", "", "")

	applyRunConfigDefaults(cmd)

	if runOpts.Image != "codex-dock:pwn" {
		t.Errorf("runOpts.Image = %q; want codex-dock:pwn", runOpts.Image)
	}
}

func TestApplyRunConfigDefaults_RunTemplateOverridesGlobal(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	prevRunOpts := runOpts
	t.Cleanup(func() { runOpts = prevRunOpts })

	runOpts.Image = "codex-dock:latest"

	viper.Set("default_template", "plain")
	viper.Set("run.template", "pwn")

	cmd := &cobra.Command{Use: "run"}
	cmd.Flags().String("image", "", "")
	cmd.Flags().Int("token-ttl", 0, "")
	cmd.Flags().String("approval-mode", "", "")
	cmd.Flags().String("user", "", "")
	cmd.Flags().String("proxy-container-url", "", "")

	applyRunConfigDefaults(cmd)

	if runOpts.Image != "codex-dock:pwn" {
		t.Errorf("runOpts.Image = %q; want codex-dock:pwn", runOpts.Image)
	}
}

func TestApplyRunConfigDefaults_ImageFlagOverridesTemplate(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	prevRunOpts := runOpts
	t.Cleanup(func() { runOpts = prevRunOpts })

	runOpts.Image = "my-image:v1"

	viper.Set("default_template", "pwn")

	cmd := &cobra.Command{Use: "run"}
	cmd.Flags().String("image", "", "")
	cmd.Flags().Int("token-ttl", 0, "")
	cmd.Flags().String("approval-mode", "", "")
	cmd.Flags().String("user", "", "")
	cmd.Flags().String("proxy-container-url", "", "")

	if err := cmd.Flags().Set("image", "my-image:v1"); err != nil {
		t.Fatalf("set image: %v", err)
	}

	applyRunConfigDefaults(cmd)

	if runOpts.Image != "my-image:v1" {
		t.Errorf("runOpts.Image = %q; want my-image:v1 (--image flag overrides template)", runOpts.Image)
	}
}

func TestApplyRunConfigDefaults_TemplateFlag(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	prevRunOpts := runOpts
	prevRunTemplate := runTemplate
	t.Cleanup(func() {
		runOpts = prevRunOpts
		runTemplate = prevRunTemplate
	})

	runOpts.Image = "codex-dock:latest"

	cmd := &cobra.Command{Use: "run"}
	cmd.Flags().String("image", "", "")
	cmd.Flags().String("template", "", "")
	cmd.Flags().Int("token-ttl", 0, "")
	cmd.Flags().String("approval-mode", "", "")
	cmd.Flags().String("user", "", "")
	cmd.Flags().String("proxy-container-url", "", "")

	if err := cmd.Flags().Set("template", "pwn"); err != nil {
		t.Fatalf("set template: %v", err)
	}
	runTemplate = "pwn"

	applyRunConfigDefaults(cmd)

	if runOpts.Image != "codex-dock:pwn" {
		t.Errorf("runOpts.Image = %q; want codex-dock:pwn", runOpts.Image)
	}
}

func TestApplyRunConfigDefaults_TemplateFlagOverridesConfig(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	prevRunOpts := runOpts
	prevRunTemplate := runTemplate
	t.Cleanup(func() {
		runOpts = prevRunOpts
		runTemplate = prevRunTemplate
	})

	runOpts.Image = "codex-dock:latest"
	viper.Set("default_template", "plain")
	viper.Set("run.image", "some-other:tag")

	cmd := &cobra.Command{Use: "run"}
	cmd.Flags().String("image", "", "")
	cmd.Flags().String("template", "", "")
	cmd.Flags().Int("token-ttl", 0, "")
	cmd.Flags().String("approval-mode", "", "")
	cmd.Flags().String("user", "", "")
	cmd.Flags().String("proxy-container-url", "", "")

	if err := cmd.Flags().Set("template", "pwn"); err != nil {
		t.Fatalf("set template: %v", err)
	}
	runTemplate = "pwn"

	applyRunConfigDefaults(cmd)

	if runOpts.Image != "codex-dock:pwn" {
		t.Errorf("runOpts.Image = %q; want codex-dock:pwn (explicit --template overrides config)", runOpts.Image)
	}
}

func TestApplyRunConfigDefaults_ImageFlagOverridesTemplateFlag(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	prevRunOpts := runOpts
	prevRunTemplate := runTemplate
	t.Cleanup(func() {
		runOpts = prevRunOpts
		runTemplate = prevRunTemplate
	})

	runOpts.Image = "my-image:v1"

	cmd := &cobra.Command{Use: "run"}
	cmd.Flags().String("image", "", "")
	cmd.Flags().String("template", "", "")
	cmd.Flags().Int("token-ttl", 0, "")
	cmd.Flags().String("approval-mode", "", "")
	cmd.Flags().String("user", "", "")
	cmd.Flags().String("proxy-container-url", "", "")

	if err := cmd.Flags().Set("image", "my-image:v1"); err != nil {
		t.Fatalf("set image: %v", err)
	}
	if err := cmd.Flags().Set("template", "pwn"); err != nil {
		t.Fatalf("set template: %v", err)
	}
	runTemplate = "pwn"

	applyRunConfigDefaults(cmd)

	if runOpts.Image != "my-image:v1" {
		t.Errorf("runOpts.Image = %q; want my-image:v1 (--image flag overrides --template flag)", runOpts.Image)
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

func TestAutoBuildEnabled_DefaultsToTrue(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	if !autoBuildEnabled() {
		t.Error("autoBuildEnabled() = false; want true when unset (default enabled)")
	}
}

func TestAutoBuildEnabled_GlobalFalse(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	viper.Set("auto_build", false)

	if autoBuildEnabled() {
		t.Error("autoBuildEnabled() = true; want false when auto_build = false")
	}
}

func TestAutoBuildEnabled_RunSectionOverridesGlobal(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	viper.Set("auto_build", true)
	viper.Set("run.auto_build", false)

	if autoBuildEnabled() {
		t.Error("autoBuildEnabled() = true; want false (run.auto_build overrides global auto_build)")
	}
}

func TestApplyRunConfigDefaults_ShellTypeFromGlobalConfig(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	prevShellTypeFlag := shellTypeFlag
	t.Cleanup(func() { shellTypeFlag = prevShellTypeFlag })
	shellTypeFlag = ""

	viper.Set("default_shell_type", "zsh")

	cmd := &cobra.Command{Use: "run"}
	cmd.Flags().String("image", "", "")
	cmd.Flags().String("shell-type", "", "")
	cmd.Flags().Int("token-ttl", 0, "")
	cmd.Flags().String("approval-mode", "", "")
	cmd.Flags().String("user", "", "")

	applyRunConfigDefaults(cmd)

	if shellTypeFlag != "zsh" {
		t.Errorf("shellTypeFlag = %q; want zsh", shellTypeFlag)
	}
}

func TestApplyRunConfigDefaults_ShellTypeRunSectionOverridesGlobal(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	prevShellTypeFlag := shellTypeFlag
	t.Cleanup(func() { shellTypeFlag = prevShellTypeFlag })
	shellTypeFlag = ""

	viper.Set("default_shell_type", "bash")
	viper.Set("run.shell_type", "zsh")

	cmd := &cobra.Command{Use: "run"}
	cmd.Flags().String("image", "", "")
	cmd.Flags().String("shell-type", "", "")
	cmd.Flags().Int("token-ttl", 0, "")
	cmd.Flags().String("approval-mode", "", "")
	cmd.Flags().String("user", "", "")

	applyRunConfigDefaults(cmd)

	if shellTypeFlag != "zsh" {
		t.Errorf("shellTypeFlag = %q; want zsh (run.shell_type overrides default_shell_type)", shellTypeFlag)
	}
}

func TestApplyRunConfigDefaults_ShellTypeFlagOverridesConfig(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	prevShellTypeFlag := shellTypeFlag
	t.Cleanup(func() { shellTypeFlag = prevShellTypeFlag })

	viper.Set("default_shell_type", "zsh")

	cmd := &cobra.Command{Use: "run"}
	cmd.Flags().String("image", "", "")
	cmd.Flags().String("shell-type", "", "")
	cmd.Flags().Int("token-ttl", 0, "")
	cmd.Flags().String("approval-mode", "", "")
	cmd.Flags().String("user", "", "")

	if err := cmd.Flags().Set("shell-type", "bash"); err != nil {
		t.Fatalf("set shell-type: %v", err)
	}
	shellTypeFlag = "bash"

	applyRunConfigDefaults(cmd)

	if shellTypeFlag != "bash" {
		t.Errorf("shellTypeFlag = %q; want bash (explicit flag overrides config)", shellTypeFlag)
	}
}

// TestResolveShellType_AutoDetectsForNormalRun verifies that, outside --shell
// mode, an empty flag value falls back to $SHELL auto-detection.
func TestResolveShellType_AutoDetectsForNormalRun(t *testing.T) {
	t.Setenv("SHELL", "/usr/bin/zsh")
	got, err := resolveShellType("", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != sandbox.ShellZsh {
		t.Errorf("resolveShellType(\"\", false) = %q; want zsh", got)
	}
}

// TestResolveShellType_ShellModeIgnoresAutoDetect verifies that --shell
// (shellMode=true) always defaults to bash when --shell-type is omitted,
// even if the host's $SHELL is zsh. --shell overrides the container
// Entrypoint directly to the shell binary (bypassing entrypoint.sh), so an
// auto-detected shell that isn't installed in the image would make Docker
// fail to start the container outright, with no way to fall back.
func TestResolveShellType_ShellModeIgnoresAutoDetect(t *testing.T) {
	t.Setenv("SHELL", "/usr/bin/zsh")
	got, err := resolveShellType("", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != sandbox.ShellBash {
		t.Errorf("resolveShellType(\"\", true) = %q; want bash (auto-detection skipped in --shell mode)", got)
	}
}

// TestResolveShellType_ShellModeHonorsExplicitFlag verifies that an explicit
// --shell-type still applies in --shell mode.
func TestResolveShellType_ShellModeHonorsExplicitFlag(t *testing.T) {
	t.Setenv("SHELL", "/bin/bash")
	got, err := resolveShellType("zsh", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != sandbox.ShellZsh {
		t.Errorf("resolveShellType(\"zsh\", true) = %q; want zsh", got)
	}
}

// TestResolveShellType_InvalidFlag verifies an unrecognized explicit value is rejected.
func TestResolveShellType_InvalidFlag(t *testing.T) {
	if _, err := resolveShellType("fish", false); err == nil {
		t.Error("expected error for invalid --shell-type, got nil")
	}
}

// TestDetectShellTypeFromEnv_Zsh verifies that a $SHELL ending in "zsh" (any
// path) resolves to ShellZsh.
func TestDetectShellTypeFromEnv_Zsh(t *testing.T) {
	t.Setenv("SHELL", "/usr/bin/zsh")
	if got := detectShellTypeFromEnv(); got != sandbox.ShellZsh {
		t.Errorf("detectShellTypeFromEnv() = %q; want zsh", got)
	}
}

// TestDetectShellTypeFromEnv_Bash verifies that $SHELL=bash resolves to
// ShellBash.
func TestDetectShellTypeFromEnv_Bash(t *testing.T) {
	t.Setenv("SHELL", "/bin/bash")
	if got := detectShellTypeFromEnv(); got != sandbox.ShellBash {
		t.Errorf("detectShellTypeFromEnv() = %q; want bash", got)
	}
}

// TestDetectShellTypeFromEnv_Unset verifies that an unset $SHELL falls back
// to ShellBash rather than erroring or leaving the type empty.
func TestDetectShellTypeFromEnv_Unset(t *testing.T) {
	t.Setenv("SHELL", "")
	if got := detectShellTypeFromEnv(); got != sandbox.ShellBash {
		t.Errorf("detectShellTypeFromEnv() = %q; want bash", got)
	}
}

// TestDetectShellTypeFromEnv_Unrecognized verifies that an unsupported shell
// (e.g. fish) falls back to ShellBash instead of propagating an invalid value.
func TestDetectShellTypeFromEnv_Unrecognized(t *testing.T) {
	t.Setenv("SHELL", "/usr/bin/fish")
	if got := detectShellTypeFromEnv(); got != sandbox.ShellBash {
		t.Errorf("detectShellTypeFromEnv() = %q; want bash", got)
	}
}

func TestResolveExecCmd_NoTrailingArgs(t *testing.T) {
	got, err := resolveExecCmd("npm test", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "npm test" {
		t.Errorf("got %q; want %q", got, "npm test")
	}
}

func TestResolveExecCmd_TrailingArgsOnly(t *testing.T) {
	got, err := resolveExecCmd("", []string{"npm", "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "npm test" {
		t.Errorf("got %q; want %q", got, "npm test")
	}
}

// TestResolveExecCmd_BothSet verifies --exec and a trailing "-- command..."
// cannot be combined.
func TestResolveExecCmd_BothSet(t *testing.T) {
	if _, err := resolveExecCmd("npm test", []string{"ls"}); err == nil {
		t.Error("expected error when --exec and trailing args are both set, got nil")
	}
}

func TestShellJoin_SimpleArgsUnquoted(t *testing.T) {
	got := shellJoin([]string{"ls", "-la", "/workspace"})
	want := "ls -la /workspace"
	if got != want {
		t.Errorf("shellJoin() = %q; want %q", got, want)
	}
}

// TestShellJoin_QuotesSpacesAndSpecialChars verifies that arguments containing
// shell metacharacters round-trip through `sh -c` as a single literal argv
// entry, so callers don't have to hand-quote multi-word commands themselves.
func TestShellJoin_QuotesSpacesAndSpecialChars(t *testing.T) {
	joined := shellJoin([]string{"echo", "hello world", "$HOME", "it's"})

	out, err := exec.Command("sh", "-c", joined+" ; true").CombinedOutput()
	if err != nil {
		t.Fatalf("sh -c %q: %v (%s)", joined, err, out)
	}

	// Re-run through a script that prints one argv entry per line so we can
	// verify the exact argument boundaries survived quoting.
	script := "print_args() { for a in \"$@\"; do echo \"[$a]\"; done; }; print_args " + shellJoin([]string{"a b", "c\"d", "it's", "$PATH"})
	out, err = exec.Command("sh", "-c", script).CombinedOutput()
	if err != nil {
		t.Fatalf("sh -c %q: %v (%s)", script, err, out)
	}
	got := strings.TrimRight(string(out), "\n")
	want := "[a b]\n[c\"d]\n[it's]\n[$PATH]"
	if got != want {
		t.Errorf("shellJoin round-trip = %q; want %q", got, want)
	}
}
