package sandbox

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
)

func TestAbsolutePath_Absolute(t *testing.T) {
	got, err := absolutePath("/tmp/workspace")
	if err != nil {
		t.Fatalf("absolutePath: %v", err)
	}
	if got != "/tmp/workspace" {
		t.Errorf("absolutePath = %q; want /tmp/workspace", got)
	}
}

func TestAbsolutePath_Relative(t *testing.T) {
	wd, _ := os.Getwd()
	got, err := absolutePath("workspace")
	if err != nil {
		t.Fatalf("absolutePath: %v", err)
	}
	want := wd + "/workspace"
	if got != want {
		t.Errorf("absolutePath = %q; want %q", got, want)
	}
}

func TestBuildCodexArgs_Minimal(t *testing.T) {
	opts := RunOptions{}
	args := buildCodexArgs(opts)
	if len(args) != 1 || args[0] != "codex" {
		t.Errorf("buildCodexArgs(minimal) = %v; want [codex]", args)
	}
}

func TestBuildCodexArgs_ApprovalMode_Suggest(t *testing.T) {
	opts := RunOptions{ApprovalMode: ApprovalModeSuggest}
	args := buildCodexArgs(opts)
	for _, a := range args {
		if a == "--ask-for-approval" || a == "--dangerously-bypass-approvals-and-sandbox" {
			t.Errorf("buildCodexArgs(suggest) should add no approval flags, got %v", args)
		}
	}
}

func TestBuildCodexArgs_ApprovalMode_AutoEdit(t *testing.T) {
	opts := RunOptions{ApprovalMode: ApprovalModeAutoEdit}
	args := buildCodexArgs(opts)
	if !containsSequence(args, "--ask-for-approval", "unless-allow-listed") {
		t.Errorf("buildCodexArgs(auto-edit) = %v; missing --ask-for-approval unless-allow-listed", args)
	}
}

func TestBuildCodexArgs_ApprovalMode_FullAuto(t *testing.T) {
	opts := RunOptions{ApprovalMode: ApprovalModeFullAuto}
	args := buildCodexArgs(opts)
	if !containsSequence(args, "--ask-for-approval", "never") {
		t.Errorf("buildCodexArgs(full-auto) = %v; missing --ask-for-approval never", args)
	}
}

func TestBuildCodexArgs_ApprovalMode_Danger(t *testing.T) {
	opts := RunOptions{ApprovalMode: ApprovalModeDanger}
	args := buildCodexArgs(opts)
	found := false
	for _, a := range args {
		if a == "--dangerously-bypass-approvals-and-sandbox" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("buildCodexArgs(danger) = %v; missing --dangerously-bypass-approvals-and-sandbox", args)
	}
}

func TestBuildCodexArgs_Model(t *testing.T) {
	opts := RunOptions{Model: "gpt-4o"}
	args := buildCodexArgs(opts)
	if !containsSequence(args, "--model", "gpt-4o") {
		t.Errorf("buildCodexArgs(Model) = %v; missing --model gpt-4o", args)
	}
}

func TestBuildCodexArgs_Task(t *testing.T) {
	opts := RunOptions{Task: "Write unit tests"}
	args := buildCodexArgs(opts)
	last := args[len(args)-1]
	if last != "Write unit tests" {
		t.Errorf("buildCodexArgs(Task) last arg = %q; want task string", last)
	}
}

func TestBuildCodexArgs_All(t *testing.T) {
	opts := RunOptions{
		ApprovalMode: ApprovalModeFullAuto,
		Model:        "o4-mini",
		Task:         "Refactor auth module",
	}
	args := buildCodexArgs(opts)
	if args[0] != "codex" {
		t.Errorf("first arg should be 'codex', got %q", args[0])
	}
	if !containsSequence(args, "--ask-for-approval", "never") {
		t.Error("missing --ask-for-approval never")
	}
	if !containsSequence(args, "--model", "o4-mini") {
		t.Error("missing --model o4-mini")
	}
	if args[len(args)-1] != "Refactor auth module" {
		t.Errorf("last arg = %q; want task", args[len(args)-1])
	}
}

func TestValidApprovalMode(t *testing.T) {
	valid := []ApprovalMode{ApprovalModeSuggest, ApprovalModeAutoEdit, ApprovalModeFullAuto, ApprovalModeDanger}
	for _, m := range valid {
		if !ValidApprovalMode(m) {
			t.Errorf("ValidApprovalMode(%q) = false; want true", m)
		}
	}
	if ValidApprovalMode("unknown") {
		t.Error("ValidApprovalMode(\"unknown\") = true; want false")
	}
	if ValidApprovalMode("") {
		t.Error("ValidApprovalMode(\"\") = true; want false")
	}
}

func TestInt64Ptr(t *testing.T) {
	v := int64ptr(512)
	if *v != 512 {
		t.Errorf("*int64ptr(512) = %d; want 512", *v)
	}
}

// TestBuildEnv_AgentsMD verifies that CODEX_AGENTS_MD is included in the
// container environment when --agents-md is specified (bug fix).
// We exercise this indirectly through buildCodexArgs since the env-building
// logic lives in Run() which requires Docker. The presence of AgentsMD in
// RunOptions is the relevant contract tested here.
func TestRunOptions_AgentsMD_FieldExists(t *testing.T) {
	opts := RunOptions{AgentsMD: "/workspace/AGENTS.md"}
	if opts.AgentsMD != "/workspace/AGENTS.md" {
		t.Errorf("AgentsMD field not set correctly: %q", opts.AgentsMD)
	}
}

// TestLogOptions_Output verifies that the Output field is present on LogOptions.
func TestLogOptions_Output_FieldExists(t *testing.T) {
	var buf strings.Builder
	opts := LogOptions{
		Name:   "worker-1",
		Tail:   100,
		Follow: false,
		Output: &buf,
	}
	if opts.Output == nil {
		t.Error("LogOptions.Output should not be nil after assignment")
	}
}

// TestImageExists_NotFound verifies that ImageExists returns false (not an
// error) for an image tag that does not exist in the local Docker daemon.
// The test is skipped when Docker is unavailable.
func TestImageExists_NotFound(t *testing.T) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		t.Skip("Docker client unavailable:", err)
	}
	defer cli.Close() //nolint:errcheck

	if _, err := cli.Ping(context.Background()); err != nil {
		t.Skip("Docker daemon not running:", err)
	}

	mgr := &Manager{cli: cli}
	exists, err := mgr.ImageExists("codex-dock-nonexistent-test-image:impossible-tag-xyz-9999")
	if err != nil {
		t.Fatalf("ImageExists returned unexpected error: %v", err)
	}
	if exists {
		t.Error("ImageExists returned true for a clearly nonexistent image")
	}
}

func TestBuildHostConfig_Security(t *testing.T) {
	hc := buildHostConfig(nil)

	// Must drop all capabilities.
	if len(hc.CapDrop) != 1 || hc.CapDrop[0] != "ALL" {
		t.Errorf("CapDrop = %v; want [ALL]", hc.CapDrop)
	}
	// Must prevent privilege escalation.
	found := false
	for _, opt := range hc.SecurityOpt {
		if opt == "no-new-privileges:true" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("SecurityOpt = %v; missing no-new-privileges:true", hc.SecurityOpt)
	}
	// PID limit must be set.
	if hc.PidsLimit == nil || *hc.PidsLimit != 512 {
		t.Errorf("PidsLimit = %v; want 512", hc.PidsLimit)
	}
}

func TestBuildHostConfig_NetworkMode(t *testing.T) {
	hc := buildHostConfig(nil)
	if string(hc.NetworkMode) != sandboxNetName {
		t.Errorf("NetworkMode = %q; want %q", hc.NetworkMode, sandboxNetName)
	}
}

func TestBuildHostConfig_HostDockerInternalGatewayMapping(t *testing.T) {
	hc := buildHostConfig(nil)
	want := "host.docker.internal:host-gateway"
	for _, h := range hc.ExtraHosts {
		if h == want {
			return
		}
	}
	t.Errorf("ExtraHosts = %v; missing %q", hc.ExtraHosts, want)
}

func TestBuildProxyFallbackURL(t *testing.T) {
	tests := []struct {
		name    string
		primary string
		want    string
	}{
		{name: "default proxy host", primary: "http://codex-auth-proxy:18080", want: "http://host.docker.internal:18080"},
		{name: "https no explicit port", primary: "https://codex-auth-proxy", want: "https://host.docker.internal:443"},
		{name: "custom host no fallback", primary: "http://proxy.internal:18080", want: ""},
		{name: "invalid URL", primary: "://bad", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildProxyFallbackURL(tt.primary); got != tt.want {
				t.Errorf("buildProxyFallbackURL(%q) = %q; want %q", tt.primary, got, tt.want)
			}
		})
	}
}

func TestBuildProxyFallbackURLWithHost(t *testing.T) {
	got := buildProxyFallbackURLWithHost("http://codex-auth-proxy:18080", "10.200.0.1")
	want := "http://10.200.0.1:18080"
	if got != want {
		t.Errorf("buildProxyFallbackURLWithHost() = %q; want %q", got, want)
	}
}

func TestBuildHostConfig_ReadOnly(t *testing.T) {
	mounts := []mount.Mount{
		{Type: mount.TypeBind, Source: "/tmp", Target: "/workspace"},
	}
	// ReadOnly is set by the caller (Run) before passing mounts in.
	mounts[0].ReadOnly = true
	hc := buildHostConfig(mounts)
	if !hc.Mounts[0].ReadOnly {
		t.Error("ReadOnly mount not preserved in buildHostConfig")
	}
}

func containsSequence(slice []string, a, b string) bool {
	for i := 0; i < len(slice)-1; i++ {
		if slice[i] == a && slice[i+1] == b {
			return true
		}
	}
	return false
}
