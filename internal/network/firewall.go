package network

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strconv"
	"strings"
)

const (
	dockerUserChain = "DOCKER-USER"
	managedChain    = "CODEX-DOCK"
)

var blockedPrivateCIDRs = []string{
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"169.254.0.0/16",
	"127.0.0.0/8",
}

var (
	ErrFirewallRootRequired     = errors.New("dock-net firewall rules require root on Linux")
	ErrFirewallIptablesNotFound = errors.New("iptables not found")
	ErrFirewallRuleNotFound     = errors.New("iptables rule not found")
	ErrFirewallChainNotFound    = errors.New("iptables chain not found")
)

type firewallController interface {
	Apply(ctx context.Context, cfg firewallConfig) error
	Remove(ctx context.Context, cfg firewallConfig) error
	Status(ctx context.Context, cfg firewallConfig) (FirewallStatus, error)
}

// FirewallStatus represents dock-net firewall installation status.
type FirewallStatus struct {
	Supported                bool
	Root                     bool
	IptablesFound            bool
	ChainExists              bool
	JumpRuleExists           bool
	DockerUserDefaultPolicy  string
	ManagedChainFinalVerdict string
}

type firewallConfig struct {
	BridgeName           string
	AllowTCPDestinations []HostEndpoint
}

// HostEndpoint is a single TCP destination that should remain reachable from workers.
type HostEndpoint struct {
	IP   string
	Port int
}

type iptablesRunner interface {
	LookPath(file string) (string, error)
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

type execIptablesRunner struct{}

func (execIptablesRunner) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

func (execIptablesRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

type iptablesFirewall struct {
	runner  iptablesRunner
	isLinux func() bool
	euid    func() int
}

func newSystemFirewall() firewallController {
	return &iptablesFirewall{
		runner:  execIptablesRunner{},
		isLinux: func() bool { return runtime.GOOS == "linux" },
		euid:    os.Geteuid,
	}
}

func (f *iptablesFirewall) Apply(ctx context.Context, cfg firewallConfig) error {
	if f.isLinux == nil {
		f.isLinux = func() bool { return runtime.GOOS == "linux" }
	}
	if f.euid == nil {
		f.euid = os.Geteuid
	}
	if !f.isLinux() {
		return nil
	}
	if f.euid() != 0 {
		if _, err := f.runner.LookPath("iptables"); err != nil {
			return fmt.Errorf("%w: %v", ErrFirewallIptablesNotFound, err)
		}
		// Avoid noisy warnings when rules are already installed by root.
		if st, err := f.Status(ctx, cfg); err == nil && st.ChainExists && st.JumpRuleExists {
			return nil
		}
		return fmt.Errorf("%w", ErrFirewallRootRequired)
	}
	if _, err := f.runner.LookPath("iptables"); err != nil {
		return fmt.Errorf("%w: %v", ErrFirewallIptablesNotFound, err)
	}

	if err := f.ensureChain(ctx, dockerUserChain); err != nil {
		return err
	}
	if err := f.ensureChain(ctx, managedChain); err != nil {
		return err
	}
	if err := f.ensureRule(ctx, dockerUserChain, []string{"-i", cfg.BridgeName, "-j", managedChain}); err != nil {
		return err
	}
	if err := f.run(ctx, "-F", managedChain); err != nil {
		return err
	}

	for _, endpoint := range normalizeHostEndpoints(cfg.AllowTCPDestinations) {
		rule := []string{
			"-A", managedChain,
			"-d", endpoint.IP + "/32",
			"-p", "tcp",
			"--dport", strconv.Itoa(endpoint.Port),
			"-m", "comment",
			"--comment", "codex-dock-allow-host",
			"-j", "RETURN",
		}
		if err := f.run(ctx, rule...); err != nil {
			return err
		}
	}

	for _, cidr := range blockedPrivateCIDRs {
		rule := []string{
			"-A", managedChain,
			"-d", cidr,
			"-m", "comment",
			"--comment", "codex-dock-drop-private",
			"-j", "DROP",
		}
		if err := f.run(ctx, rule...); err != nil {
			return err
		}
	}

	return f.run(ctx, "-A", managedChain, "-j", "RETURN")
}

func (f *iptablesFirewall) Remove(ctx context.Context, cfg firewallConfig) error {
	if f.isLinux == nil {
		f.isLinux = func() bool { return runtime.GOOS == "linux" }
	}
	if f.euid == nil {
		f.euid = os.Geteuid
	}
	if !f.isLinux() {
		return nil
	}
	if f.euid() != 0 {
		return fmt.Errorf("%w", ErrFirewallRootRequired)
	}
	if _, err := f.runner.LookPath("iptables"); err != nil {
		return fmt.Errorf("%w: %v", ErrFirewallIptablesNotFound, err)
	}

	rule := []string{"-i", cfg.BridgeName, "-j", managedChain}
	for {
		if err := f.deleteRule(ctx, dockerUserChain, rule); err != nil {
			if errors.Is(err, ErrFirewallRuleNotFound) || errors.Is(err, ErrFirewallChainNotFound) {
				break
			}
			return err
		}
	}

	if err := f.runMaybeMissing(ctx, "-F", managedChain); err != nil {
		if !errors.Is(err, ErrFirewallChainNotFound) {
			return err
		}
	}
	if err := f.runMaybeMissing(ctx, "-X", managedChain); err != nil {
		if !errors.Is(err, ErrFirewallChainNotFound) {
			return err
		}
	}
	return nil
}

func (f *iptablesFirewall) Status(ctx context.Context, cfg firewallConfig) (FirewallStatus, error) {
	if f.isLinux == nil {
		f.isLinux = func() bool { return runtime.GOOS == "linux" }
	}
	if f.euid == nil {
		f.euid = os.Geteuid
	}

	status := FirewallStatus{
		Supported: f.isLinux(),
		Root:      f.euid() == 0,
	}
	if !status.Supported {
		return status, nil
	}
	if _, err := f.runner.LookPath("iptables"); err != nil {
		return status, nil
	}
	status.IptablesFound = true

	if err := f.runMaybeMissing(ctx, "-S", managedChain); err == nil {
		status.ChainExists = true
		if out, err := f.runOutput(ctx, "-S", managedChain); err == nil {
			status.ManagedChainFinalVerdict = managedChainFinalVerdict(out)
		}
	} else if !errors.Is(err, ErrFirewallChainNotFound) {
		return status, err
	}

	if out, err := f.runMaybeMissingOutput(ctx, "-S", dockerUserChain); err == nil {
		status.DockerUserDefaultPolicy = chainDefaultPolicy(out, dockerUserChain)
	} else if !errors.Is(err, ErrFirewallChainNotFound) {
		return status, err
	}

	rule := []string{"-C", dockerUserChain, "-i", cfg.BridgeName, "-j", managedChain}
	if err := f.runMaybeMissing(ctx, rule...); err == nil {
		status.JumpRuleExists = true
	} else if !errors.Is(err, ErrFirewallRuleNotFound) && !errors.Is(err, ErrFirewallChainNotFound) {
		return status, err
	}

	return status, nil
}

func (f *iptablesFirewall) ensureChain(ctx context.Context, chain string) error {
	if err := f.runMaybeMissing(ctx, "-S", chain); err != nil {
		if !errors.Is(err, ErrFirewallChainNotFound) {
			return err
		}
		return f.run(ctx, "-N", chain)
	}
	return nil
}

func (f *iptablesFirewall) ensureRule(ctx context.Context, chain string, rule []string) error {
	checkArgs := append([]string{"-C", chain}, rule...)
	if err := f.runMaybeMissing(ctx, checkArgs...); err == nil {
		return nil
	} else if !errors.Is(err, ErrFirewallRuleNotFound) {
		return err
	}

	insertArgs := append([]string{"-I", chain, "1"}, rule...)
	return f.run(ctx, insertArgs...)
}

func (f *iptablesFirewall) deleteRule(ctx context.Context, chain string, rule []string) error {
	args := append([]string{"-D", chain}, rule...)
	return f.runMaybeMissing(ctx, args...)
}

func (f *iptablesFirewall) run(ctx context.Context, args ...string) error {
	_, err := f.runOutput(ctx, args...)
	return err
}

func (f *iptablesFirewall) runMaybeMissing(ctx context.Context, args ...string) error {
	_, err := f.runOutput(ctx, args...)
	return err
}

func (f *iptablesFirewall) runMaybeMissingOutput(ctx context.Context, args ...string) ([]byte, error) {
	return f.runOutput(ctx, args...)
}

func (f *iptablesFirewall) runOutput(ctx context.Context, args ...string) ([]byte, error) {
	out, err := f.runner.Run(ctx, "iptables", args...)
	if err == nil {
		return out, nil
	}
	if errors.Is(err, ErrFirewallRuleNotFound) || errors.Is(err, ErrFirewallChainNotFound) {
		return out, err
	}

	msg := strings.ToLower(string(bytes.TrimSpace(out)))
	switch {
	case strings.Contains(msg, "no chain/target/match by that name"):
		return out, ErrFirewallChainNotFound
	case strings.Contains(msg, "bad rule"):
		return out, ErrFirewallRuleNotFound
	case strings.Contains(msg, "does a matching rule exist in that chain"):
		return out, ErrFirewallRuleNotFound
	case strings.Contains(msg, "chain '") && strings.Contains(msg, "does not exist"):
		return out, ErrFirewallChainNotFound
	case strings.Contains(msg, "no such file or directory"):
		return out, ErrFirewallChainNotFound
	case strings.Contains(msg, "permission denied"):
		return out, fmt.Errorf("iptables command failed: permission denied (run codex-dock as root): %w", err)
	default:
		return out, fmt.Errorf("iptables %s failed: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
}

func chainDefaultPolicy(out []byte, chain string) string {
	prefix := "-P " + chain + " "
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}

func managedChainFinalVerdict(out []byte) string {
	lastJump := ""
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "-A "+managedChain+" ") {
			if strings.Contains(line, " -j ") {
				lastJump = line
			}
		}
	}
	if lastJump == "" {
		return ""
	}
	idx := strings.LastIndex(lastJump, " -j ")
	if idx < 0 {
		return ""
	}
	return strings.TrimSpace(lastJump[idx+4:])
}

func IsFirewallWarning(err error) bool {
	return errors.Is(err, ErrFirewallRootRequired) || errors.Is(err, ErrFirewallIptablesNotFound)
}

func normalizeHostEndpoints(endpoints []HostEndpoint) []HostEndpoint {
	seen := map[string]struct{}{}
	normalized := make([]HostEndpoint, 0, len(endpoints))

	for _, endpoint := range endpoints {
		ip := strings.TrimSpace(endpoint.IP)
		if net.ParseIP(ip) == nil || endpoint.Port <= 0 || endpoint.Port > 65535 {
			continue
		}
		key := ip + ":" + strconv.Itoa(endpoint.Port)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, HostEndpoint{IP: ip, Port: endpoint.Port})
	}

	slices.SortFunc(normalized, func(a, b HostEndpoint) int {
		if a.IP != b.IP {
			return strings.Compare(a.IP, b.IP)
		}
		return a.Port - b.Port
	})
	return normalized
}
