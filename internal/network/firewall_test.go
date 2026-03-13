package network

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestIsFirewallWarning(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "root required", err: ErrFirewallRootRequired, want: true},
		{name: "iptables missing", err: ErrFirewallIptablesNotFound, want: true},
		{name: "wrapped root", err: errors.New("noop"), want: true},
		{name: "rule not found", err: ErrFirewallRuleNotFound, want: false},
		{name: "nil", err: nil, want: false},
	}

	tests[2].err = errors.Join(errors.New("wrapped"), ErrFirewallRootRequired)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsFirewallWarning(tt.err); got != tt.want {
				t.Fatalf("IsFirewallWarning(%v)=%v want=%v", tt.err, got, tt.want)
			}
		})
	}
}

func TestIptablesFirewallApplyReturnsRootRequiredOnLinux(t *testing.T) {
	fw := &iptablesFirewall{
		runner:  &stubIptablesRunner{},
		isLinux: func() bool { return true },
		euid:    func() int { return 1000 },
	}

	err := fw.Apply(context.Background(), firewallConfig{BridgeName: BridgeName})
	if !errors.Is(err, ErrFirewallRootRequired) {
		t.Fatalf("Apply() err=%v want ErrFirewallRootRequired", err)
	}
}

func TestNormalizeHostEndpoints(t *testing.T) {
	got := normalizeHostEndpoints([]HostEndpoint{
		{IP: "10.0.0.5", Port: 18080},
		{IP: "10.0.0.5", Port: 18080},
		{IP: "bad-ip", Port: 80},
		{IP: "192.168.0.10", Port: 70000},
		{IP: "192.168.0.10", Port: 443},
	})

	want := []HostEndpoint{
		{IP: "10.0.0.5", Port: 18080},
		{IP: "192.168.0.10", Port: 443},
	}
	if len(got) != len(want) {
		t.Fatalf("normalizeHostEndpoints() len=%d want=%d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("normalizeHostEndpoints()[%d]=%+v want=%+v", i, got[i], want[i])
		}
	}
}

func TestIptablesFirewallApplyBuildsRules(t *testing.T) {
	runner := &stubIptablesRunner{
		chainErrors: map[string]error{
			dockerUserChain: nil,
			managedChain:    ErrFirewallChainNotFound,
		},
	}
	fw := &iptablesFirewall{
		runner:  runner,
		isLinux: func() bool { return true },
		euid:    func() int { return 0 },
	}

	err := fw.Apply(context.Background(), firewallConfig{
		BridgeName: BridgeName,
		AllowTCPDestinations: []HostEndpoint{
			{IP: "172.17.0.1", Port: 18080},
		},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	got := strings.Join(runner.calls, "\n")
	for _, want := range []string{
		"iptables -S DOCKER-USER",
		"iptables -S CODEX-DOCK",
		"iptables -N CODEX-DOCK",
		"iptables -C DOCKER-USER -i dock-net0 -j CODEX-DOCK",
		"iptables -I DOCKER-USER 1 -i dock-net0 -j CODEX-DOCK",
		"iptables -F CODEX-DOCK",
		"iptables -A CODEX-DOCK -d 172.17.0.1/32 -p tcp --dport 18080 -m comment --comment codex-dock-allow-host -j RETURN",
		"iptables -A CODEX-DOCK -d 10.0.0.0/8 -m comment --comment codex-dock-drop-private -j DROP",
		"iptables -A CODEX-DOCK -j RETURN",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Apply() missing command %q\ncalls:\n%s", want, got)
		}
	}
}

type stubIptablesRunner struct {
	calls       []string
	chainErrors map[string]error
}

func (s *stubIptablesRunner) LookPath(file string) (string, error) {
	return "/usr/sbin/" + file, nil
}

func (s *stubIptablesRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	s.calls = append(s.calls, name+" "+strings.Join(args, " "))
	if len(args) >= 2 && args[0] == "-S" {
		if err, ok := s.chainErrors[args[1]]; ok {
			return nil, err
		}
	}
	if len(args) >= 2 && args[0] == "-C" {
		return nil, ErrFirewallRuleNotFound
	}
	return nil, nil
}

func TestIptablesFirewallRemoveIgnoresMissingManagedChain(t *testing.T) {
	runner := &removeRunner{}
	fw := &iptablesFirewall{
		runner:  runner,
		isLinux: func() bool { return true },
		euid:    func() int { return 0 },
	}

	err := fw.Remove(context.Background(), firewallConfig{BridgeName: BridgeName})
	if err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if len(runner.calls) == 0 {
		t.Fatal("Remove() did not execute any commands")
	}
}

type removeRunner struct {
	calls []string
}

func (r *removeRunner) LookPath(file string) (string, error) {
	return "/usr/sbin/" + file, nil
}

func (r *removeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, name+" "+strings.Join(args, " "))
	if len(args) >= 1 && args[0] == "-D" {
		return nil, ErrFirewallRuleNotFound
	}
	if len(args) >= 2 && (args[0] == "-F" || args[0] == "-X") {
		return nil, ErrFirewallChainNotFound
	}
	return nil, nil
}

func TestIptablesFirewallRunOutputClassifiesErrors(t *testing.T) {
	runner := &errorRunner{
		err:    errors.New("exit status 1"),
		stdout: []byte("iptables: Bad rule"),
	}
	fw := &iptablesFirewall{
		runner:  runner,
		isLinux: func() bool { return true },
		euid:    func() int { return 0 },
	}

	_, err := fw.runOutput(context.Background(), "-C", managedChain, "-j", "RETURN")
	if !errors.Is(err, ErrFirewallRuleNotFound) {
		t.Fatalf("runOutput() err = %v; want ErrFirewallRuleNotFound", err)
	}
}

type errorRunner struct {
	err    error
	stdout []byte
}

func (r *errorRunner) LookPath(file string) (string, error) {
	return "/usr/sbin/" + file, nil
}

func (r *errorRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	return r.stdout, r.err
}

func TestIptablesFirewallStatus(t *testing.T) {
	runner := &statusRunner{}
	fw := &iptablesFirewall{
		runner:  runner,
		isLinux: func() bool { return true },
		euid:    func() int { return 0 },
	}

	st, err := fw.Status(context.Background(), firewallConfig{BridgeName: BridgeName})
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if !st.Supported || !st.Root || !st.IptablesFound || !st.ChainExists || !st.JumpRuleExists {
		t.Fatalf("unexpected status: %+v", st)
	}
}

func TestIptablesFirewallStatusMissingIptables(t *testing.T) {
	fw := &iptablesFirewall{
		runner:  missingIptablesRunner{},
		isLinux: func() bool { return true },
		euid:    func() int { return 1000 },
	}

	st, err := fw.Status(context.Background(), firewallConfig{BridgeName: BridgeName})
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if !st.Supported || st.IptablesFound {
		t.Fatalf("unexpected status: %+v", st)
	}
}

type missingIptablesRunner struct{}

type statusRunner struct{}

func (statusRunner) LookPath(file string) (string, error) {
	return "/usr/sbin/" + file, nil
}

func (statusRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	return nil, nil
}

func (missingIptablesRunner) LookPath(file string) (string, error) {
	return "", errors.New("not found")
}

func (missingIptablesRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	return nil, nil
}
