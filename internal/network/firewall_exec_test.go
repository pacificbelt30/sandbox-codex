package network

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// End-to-end verification of the real exec path (execIptablesRunner) for the
// --sudo feature. The portable tests drive Apply against fake sudo/iptables
// shell scripts on PATH and assert how commands are constructed; they run on
// any Linux host. TestReal_* exercises real iptables and is gated behind an
// env var (requires root) so it is skipped in normal CI runs.

func writeFakeBin(t *testing.T, dir, name, body string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func setupFakeBins(t *testing.T) (sudoLog, iptLog string) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("firewall exec path is Linux-only")
	}
	dir := t.TempDir()
	sudoLog = filepath.Join(dir, "sudo.log")
	iptLog = filepath.Join(dir, "ipt.log")
	t.Setenv("SUDO_LOG", sudoLog)
	t.Setenv("IPT_LOG", iptLog)

	// fake iptables: mimic real "bad rule" on -C so check/delete loops end.
	writeFakeBin(t, dir, "iptables", `#!/bin/sh
echo "iptables $*" >> "$IPT_LOG"
for a in "$@"; do
  if [ "$a" = "-C" ]; then
    echo "iptables: Bad rule (does a matching rule exist in that chain?)." 1>&2
    exit 1
  fi
done
exit 0
`)
	// fake sudo: log, then -v succeeds, -n execs the wrapped command.
	writeFakeBin(t, dir, "sudo", `#!/bin/sh
echo "sudo $*" >> "$SUDO_LOG"
if [ "$1" = "-v" ]; then exit 0; fi
if [ "$1" = "-n" ]; then shift; exec "$@"; fi
exit 0
`)

	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return sudoLog, iptLog
}

func readLog(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		t.Fatalf("read %s: %v", p, err)
	}
	return string(b)
}

func TestE2E_NonRootSudo_WrapsRealExec(t *testing.T) {
	sudoLog, iptLog := setupFakeBins(t)

	fw := &iptablesFirewall{
		runner:      execIptablesRunner{},
		isLinux:     func() bool { return true },
		euid:        func() int { return 1000 },
		interactive: func() bool { return false }, // non-interactive: no prime
	}

	err := fw.Apply(context.Background(), firewallConfig{
		BridgeName:        BridgeName,
		BlockDestinations: []BlockDestination{{CIDR: "203.0.113.0/24"}},
		UseSudo:           true,
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	sudo := readLog(t, sudoLog)
	ipt := readLog(t, iptLog)
	t.Logf("SUDO LOG:\n%s", sudo)
	t.Logf("IPT LOG:\n%s", ipt)

	if !strings.Contains(sudo, "sudo -n iptables -A CODEX-DOCK -d 203.0.113.0/24") {
		t.Fatalf("expected sudo-wrapped block rule in sudo log:\n%s", sudo)
	}
	if !strings.Contains(sudo, "sudo -n iptables -A CODEX-DOCK -d 10.0.0.0/8") {
		t.Fatalf("expected sudo-wrapped private drop in sudo log:\n%s", sudo)
	}
	// Non-interactive => sudo -v must NOT have been called.
	if strings.Contains(sudo, "sudo -v") {
		t.Fatalf("non-interactive prime should not call sudo -v:\n%s", sudo)
	}
	// The wrapped iptables actually ran (via exec from fake sudo).
	if !strings.Contains(ipt, "iptables -A CODEX-DOCK -d 203.0.113.0/24") {
		t.Fatalf("wrapped iptables did not execute:\n%s", ipt)
	}
}

func TestE2E_InteractiveSudo_PrimesWithDashV(t *testing.T) {
	sudoLog, _ := setupFakeBins(t)

	fw := &iptablesFirewall{
		runner:      execIptablesRunner{},
		isLinux:     func() bool { return true },
		euid:        func() int { return 1000 },
		interactive: func() bool { return true }, // interactive: prime via sudo -v
	}

	if err := fw.Apply(context.Background(), firewallConfig{BridgeName: BridgeName, UseSudo: true}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	sudo := readLog(t, sudoLog)
	if !strings.Contains(sudo, "sudo -v") {
		t.Fatalf("interactive prime should call sudo -v exactly once:\n%s", sudo)
	}
	if got := strings.Count(sudo, "sudo -v"); got != 1 {
		t.Fatalf("sudo -v called %d times; want 1:\n%s", got, sudo)
	}
}

func TestE2E_Root_NoSudoWrap(t *testing.T) {
	sudoLog, iptLog := setupFakeBins(t)

	fw := &iptablesFirewall{
		runner:  execIptablesRunner{},
		isLinux: func() bool { return true },
		euid:    func() int { return 0 }, // root: sudo ignored even if UseSudo
	}

	if err := fw.Apply(context.Background(), firewallConfig{BridgeName: BridgeName, UseSudo: true}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	if s := readLog(t, sudoLog); s != "" {
		t.Fatalf("root path must not invoke sudo, got:\n%s", s)
	}
	if ipt := readLog(t, iptLog); !strings.Contains(ipt, "iptables -A CODEX-DOCK -d 10.0.0.0/8") {
		t.Fatalf("root path should call iptables directly:\n%s", ipt)
	}
}

func TestReal_ApplyStatusRemove_RoundTrip(t *testing.T) {
	if os.Getenv("CODEX_DOCK_REAL_IPTABLES") != "1" {
		t.Skip("set CODEX_DOCK_REAL_IPTABLES=1 to run against real iptables")
	}
	fw := newSystemFirewall()
	cfg := firewallConfig{
		BridgeName:           "dock-net0",
		BridgeSubnet:         "10.200.0.0/24",
		AllowTCPDestinations: []HostEndpoint{{IP: "172.17.0.1", Port: 18080}},
		BlockDestinations:    []BlockDestination{{CIDR: "203.0.113.0/24"}, {CIDR: "198.51.100.5/32", Port: 443}},
	}
	ctx := context.Background()

	if err := fw.Apply(ctx, cfg); err != nil {
		t.Fatalf("real Apply() error = %v", err)
	}
	st, err := fw.Status(ctx, cfg)
	if err != nil {
		t.Fatalf("real Status() error = %v", err)
	}
	t.Logf("status: chain=%v jump=%v finalVerdict=%s rules=%d", st.ChainExists, st.JumpRuleExists, st.ManagedChainFinalVerdict, len(st.Rules))
	for _, r := range st.Rules {
		t.Logf("  %-5s %-18s port=%d %s", r.Action, r.Destination, r.Port, r.Comment)
	}
	if !st.ChainExists || !st.JumpRuleExists {
		t.Fatalf("expected chain+jump installed: %+v", st)
	}
	if err := fw.Remove(ctx, cfg); err != nil {
		t.Fatalf("real Remove() error = %v", err)
	}
	st2, err := fw.Status(ctx, cfg)
	if err != nil {
		t.Fatalf("real Status() after remove error = %v", err)
	}
	if st2.JumpRuleExists {
		t.Fatalf("jump rule should be gone after Remove: %+v", st2)
	}
}
