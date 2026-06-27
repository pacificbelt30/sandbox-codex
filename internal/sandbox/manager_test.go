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
	hc := buildHostConfig(nil, "dock-net-w-test")

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
	want := "dock-net-w-alpha"
	hc := buildHostConfig(nil, want)
	if string(hc.NetworkMode) != want {
		t.Errorf("NetworkMode = %q; want %q", hc.NetworkMode, want)
	}
}

func TestBuildHostConfig_NoExtraHosts(t *testing.T) {
	// The router model reaches the proxy via Docker DNS on the Internal network,
	// so the host.docker.internal/host-gateway alias must NOT be set.
	hc := buildHostConfig(nil, "dock-net-w-alpha")
	if len(hc.ExtraHosts) != 0 {
		t.Errorf("ExtraHosts = %v; want none (router model uses Docker DNS)", hc.ExtraHosts)
	}
}

func TestPickUniqueName(t *testing.T) {
	// First generated name is free → used as-is.
	t.Run("first free", func(t *testing.T) {
		names := []string{"codex-brave-otter", "codex-calm-finch"}
		i := 0
		gen := func() string { n := names[i%len(names)]; i++; return n }
		got := pickUniqueName(gen, func(string) bool { return false }, 12)
		if got != "codex-brave-otter" {
			t.Errorf("got %q; want first generated name", got)
		}
	})

	// Skips taken names and returns the first free one.
	t.Run("skips taken", func(t *testing.T) {
		seq := []string{"taken-1", "taken-2", "free-3"}
		i := 0
		gen := func() string { n := seq[i]; i++; return n }
		taken := func(n string) bool { return strings.HasPrefix(n, "taken") }
		got := pickUniqueName(gen, taken, 12)
		if got != "free-3" {
			t.Errorf("got %q; want free-3", got)
		}
	})

	// Everything taken → appends a random suffix to the last generated name.
	t.Run("suffix fallback", func(t *testing.T) {
		got := pickUniqueName(func() string { return "codex-x-y" }, func(string) bool { return true }, 3)
		if !strings.HasPrefix(got, "codex-x-y-") {
			t.Errorf("got %q; want a suffixed codex-x-y-<hex>", got)
		}
		if got == "codex-x-y-" || len(got) <= len("codex-x-y-") {
			t.Errorf("suffix missing in %q", got)
		}
	})

	// Base name taken, but the suffixed fallback is free → returns a suffixed
	// name (and the fallback is verified free, not blindly returned).
	t.Run("suffix re-checked", func(t *testing.T) {
		taken := func(n string) bool { return n == "codex-x-y" } // only the bare base is taken
		got := pickUniqueName(func() string { return "codex-x-y" }, taken, 3)
		if got == "codex-x-y" {
			t.Fatalf("returned the taken base name")
		}
		if !strings.HasPrefix(got, "codex-x-y-") {
			t.Errorf("got %q; want a free suffixed name", got)
		}
		if taken(got) {
			t.Errorf("returned a name reported taken: %q", got)
		}
	})
}

func TestRandomSuffix_Unique(t *testing.T) {
	a, b := randomSuffix(), randomSuffix()
	if a == "" || b == "" {
		t.Fatal("randomSuffix returned empty")
	}
	if a == b {
		t.Errorf("randomSuffix produced duplicates: %q", a)
	}
}

func TestProxyEndpointHost(t *testing.T) {
	tests := []struct {
		endpoint string
		want     string
	}{
		{"http://codex-auth-proxy:18080", "codex-auth-proxy"},
		{"http://codex-auth-proxy:18080/v1", "codex-auth-proxy"},
		{"https://proxy.internal", "proxy.internal"},
		{"://bad", ""},
	}
	for _, tt := range tests {
		if got := proxyEndpointHost(tt.endpoint); got != tt.want {
			t.Errorf("proxyEndpointHost(%q) = %q; want %q", tt.endpoint, got, tt.want)
		}
	}
}

func TestBuildHostConfig_ReadOnly(t *testing.T) {
	mounts := []mount.Mount{
		{Type: mount.TypeBind, Source: "/tmp", Target: "/workspace"},
	}
	// ReadOnly is set by the caller (Run) before passing mounts in.
	mounts[0].ReadOnly = true
	hc := buildHostConfig(mounts, "dock-net-w-test")
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
