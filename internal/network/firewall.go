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
	ErrFirewallPermissionDenied = errors.New("iptables permission denied")
	ErrFirewallSudoNotFound     = errors.New("sudo not found (required by --sudo)")
	ErrFirewallSudoFailed       = errors.New("sudo authentication failed")
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
	Rules                    []FirewallRule
}

// FirewallRule is a single parsed rule from the managed CODEX-DOCK chain,
// in evaluation order.
type FirewallRule struct {
	// Action is "allow" (RETURN/ACCEPT) or "block" (DROP).
	Action string
	// Verdict is the raw iptables target: RETURN, ACCEPT, or DROP.
	Verdict string
	// Destination is the -d CIDR/IP (empty means "any").
	Destination string
	// Protocol is the -p value (e.g. "tcp"), empty means any protocol.
	Protocol string
	// Port is the --dport value, 0 when the rule is not port-scoped.
	Port int
	// Comment is the rule's --comment tag, used to derive a friendly label.
	Comment string
}

type firewallConfig struct {
	BridgeName           string
	ProxyBridgeName      string
	BridgeSubnet         string
	AllowTCPPorts        []int
	AllowTCPDestinations []HostEndpoint
	BlockDestinations    []BlockDestination
	// UseSudo runs iptables via sudo when the current process is not root.
	UseSudo bool
}

// HostEndpoint is a single TCP destination that should remain reachable from workers.
type HostEndpoint struct {
	IP   string
	Port int
}

// BlockDestination is an extra destination that should be dropped for workers.
// CIDR is a canonical IPv4 network (e.g. "203.0.113.0/24" or "203.0.113.10/32").
// Port is 0 to drop all protocols/ports, or a TCP port to drop only that port.
type BlockDestination struct {
	CIDR string
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
	// interactive reports whether stdin is a terminal, so sudo may prompt.
	interactive func() bool
	// primeSudo refreshes sudo credentials once before iptables calls run.
	primeSudo func(ctx context.Context) error

	// sudo and primed are per-invocation state populated from firewallConfig.
	sudo   bool
	primed bool
}

func newSystemFirewall() firewallController {
	return &iptablesFirewall{
		runner:  execIptablesRunner{},
		isLinux: func() bool { return runtime.GOOS == "linux" },
		euid:    os.Geteuid,
	}
}

// defaultInteractive reports whether stdin is attached to a terminal, which is
// the signal we use to decide whether sudo may interactively prompt for a
// password.
func defaultInteractive() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// initFuncs fills in default implementations for the injectable hooks so tests
// that construct an iptablesFirewall with only runner/isLinux/euid keep working.
func (f *iptablesFirewall) initFuncs() {
	if f.isLinux == nil {
		f.isLinux = func() bool { return runtime.GOOS == "linux" }
	}
	if f.euid == nil {
		f.euid = os.Geteuid
	}
	if f.interactive == nil {
		f.interactive = defaultInteractive
	}
	if f.primeSudo == nil {
		f.primeSudo = f.defaultPrimeSudo
	}
}

// defaultPrimeSudo establishes a sudo session before the firewall issues its
// `sudo -n iptables` calls. In an interactive terminal it runs `sudo -v`, which
// prompts for a password once and caches the credentials so the subsequent
// non-interactive calls succeed. In a non-interactive environment it never
// prompts: it relies on cached credentials or a NOPASSWD sudoers entry, and any
// failure surfaces on the first real iptables call instead of hanging.
func (f *iptablesFirewall) defaultPrimeSudo(ctx context.Context) error {
	if !f.interactive() {
		return nil
	}
	cmd := exec.CommandContext(ctx, "sudo", "-v")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %v", ErrFirewallSudoFailed, err)
	}
	return nil
}

// useSudo reports whether iptables invocations should be wrapped in sudo. It is
// only meaningful for a non-root process that opted in via --sudo.
func (f *iptablesFirewall) useSudo() bool {
	return f.sudo && f.euid() != 0
}

// ensureSudoPrimed primes the sudo session at most once per invocation.
func (f *iptablesFirewall) ensureSudoPrimed(ctx context.Context) error {
	if !f.useSudo() || f.primed {
		return nil
	}
	if err := f.primeSudo(ctx); err != nil {
		return err
	}
	f.primed = true
	return nil
}

// ensureToolPath verifies the binary the firewall is about to invoke exists.
// When running via sudo we only require sudo on PATH; iptables frequently lives
// in a directory (e.g. /usr/sbin) that is not on an unprivileged user's PATH,
// so its presence is validated by the first `sudo iptables` call instead.
func (f *iptablesFirewall) ensureToolPath() error {
	if f.useSudo() {
		if _, err := f.runner.LookPath("sudo"); err != nil {
			return fmt.Errorf("%w: %v", ErrFirewallSudoNotFound, err)
		}
		return nil
	}
	if _, err := f.runner.LookPath("iptables"); err != nil {
		return fmt.Errorf("%w: %v", ErrFirewallIptablesNotFound, err)
	}
	return nil
}

func (f *iptablesFirewall) Apply(ctx context.Context, cfg firewallConfig) error {
	f.initFuncs()
	if !f.isLinux() {
		return nil
	}
	f.sudo = cfg.UseSudo

	if f.euid() != 0 && !f.sudo {
		// Non-root without --sudo: keep the existing best-effort behavior.
		if _, err := f.runner.LookPath("iptables"); err != nil {
			return fmt.Errorf("%w: %v", ErrFirewallIptablesNotFound, err)
		}
		// Avoid noisy warnings when rules are already installed by root.
		if st, err := f.Status(ctx, cfg); err == nil {
			if st.ChainExists && st.JumpRuleExists {
				return nil
			}
		} else if errors.Is(err, ErrFirewallPermissionDenied) {
			// Some environments disallow non-root iptables reads.
			// Do not emit false root-required warnings in that case.
			return nil
		}
		return fmt.Errorf("%w", ErrFirewallRootRequired)
	}

	// Root, or a non-root process that opted into sudo.
	if err := f.ensureToolPath(); err != nil {
		return err
	}
	if err := f.ensureSudoPrimed(ctx); err != nil {
		return err
	}

	if err := f.ensureChain(ctx, dockerUserChain); err != nil {
		return err
	}
	if err := f.ensureChain(ctx, managedChain); err != nil {
		return err
	}

	for _, port := range normalizePorts(cfg.AllowTCPPorts) {
		if cfg.ProxyBridgeName == "" {
			continue
		}
		if err := f.ensureRule(ctx, dockerUserChain, []string{"-i", cfg.BridgeName, "-o", cfg.ProxyBridgeName, "-p", "tcp", "--dport", strconv.Itoa(port), "-j", "ACCEPT"}); err != nil {
			return err
		}
	}

	if cfg.ProxyBridgeName != "" {
		if err := f.ensureRule(ctx, dockerUserChain, []string{"-i", cfg.ProxyBridgeName, "-o", cfg.BridgeName, "-m", "conntrack", "--ctstate", "ESTABLISHED,RELATED", "-j", "ACCEPT"}); err != nil {
			return err
		}
	}

	if err := f.ensureRuleLast(ctx, dockerUserChain, []string{"-i", cfg.BridgeName, "-j", managedChain}); err != nil {
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

	if cfg.BridgeSubnet != "" {
		for _, port := range normalizePorts(cfg.AllowTCPPorts) {
			rule := []string{
				"-A", managedChain,
				"-d", cfg.BridgeSubnet,
				"-p", "tcp",
				"--dport", strconv.Itoa(port),
				"-m", "comment",
				"--comment", "codex-dock-allow-bridge-subnet",
				"-j", "RETURN",
			}
			if err := f.run(ctx, rule...); err != nil {
				return err
			}
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

	for _, block := range normalizeBlockDestinations(cfg.BlockDestinations) {
		rule := []string{"-A", managedChain, "-d", block.CIDR}
		if block.Port > 0 {
			rule = append(rule, "-p", "tcp", "--dport", strconv.Itoa(block.Port))
		}
		rule = append(rule, "-m", "comment", "--comment", "codex-dock-block-host", "-j", "DROP")
		if err := f.run(ctx, rule...); err != nil {
			return err
		}
	}

	return f.run(ctx, "-A", managedChain, "-j", "RETURN")
}

func (f *iptablesFirewall) Remove(ctx context.Context, cfg firewallConfig) error {
	f.initFuncs()
	if !f.isLinux() {
		return nil
	}
	f.sudo = cfg.UseSudo
	if f.euid() != 0 && !f.sudo {
		return fmt.Errorf("%w", ErrFirewallRootRequired)
	}
	if err := f.ensureToolPath(); err != nil {
		return err
	}
	if err := f.ensureSudoPrimed(ctx); err != nil {
		return err
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

	if cfg.ProxyBridgeName != "" {
		reverseRule := []string{"-i", cfg.ProxyBridgeName, "-o", cfg.BridgeName, "-m", "conntrack", "--ctstate", "ESTABLISHED,RELATED", "-j", "ACCEPT"}
		for {
			if err := f.deleteRule(ctx, dockerUserChain, reverseRule); err != nil {
				if errors.Is(err, ErrFirewallRuleNotFound) || errors.Is(err, ErrFirewallChainNotFound) {
					break
				}
				return err
			}
		}

		for _, port := range normalizePorts(cfg.AllowTCPPorts) {
			forwardRule := []string{"-i", cfg.BridgeName, "-o", cfg.ProxyBridgeName, "-p", "tcp", "--dport", strconv.Itoa(port), "-j", "ACCEPT"}
			for {
				if err := f.deleteRule(ctx, dockerUserChain, forwardRule); err != nil {
					if errors.Is(err, ErrFirewallRuleNotFound) || errors.Is(err, ErrFirewallChainNotFound) {
						break
					}
					return err
				}
			}
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
	f.initFuncs()
	f.sudo = cfg.UseSudo

	status := FirewallStatus{
		Supported: f.isLinux(),
		Root:      f.euid() == 0,
	}
	if !status.Supported {
		return status, nil
	}
	if f.useSudo() {
		if _, err := f.runner.LookPath("sudo"); err != nil {
			return status, nil
		}
	} else if _, err := f.runner.LookPath("iptables"); err != nil {
		return status, nil
	}
	status.IptablesFound = true
	if err := f.ensureSudoPrimed(ctx); err != nil {
		return status, err
	}

	if err := f.runMaybeMissing(ctx, "-S", managedChain); err == nil {
		status.ChainExists = true
		if out, err := f.runOutput(ctx, "-S", managedChain); err == nil {
			status.ManagedChainFinalVerdict = managedChainFinalVerdict(out)
			status.Rules = parseManagedChainRules(out)
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

func (f *iptablesFirewall) ensureRuleLast(ctx context.Context, chain string, rule []string) error {
	checkArgs := append([]string{"-C", chain}, rule...)
	for {
		err := f.runMaybeMissing(ctx, checkArgs...)
		if err == nil {
			if err := f.deleteRule(ctx, chain, rule); err != nil {
				if errors.Is(err, ErrFirewallRuleNotFound) || errors.Is(err, ErrFirewallChainNotFound) {
					break
				}
				return err
			}
			continue
		}
		if errors.Is(err, ErrFirewallRuleNotFound) || errors.Is(err, ErrFirewallChainNotFound) {
			break
		}
		return err
	}

	appendArgs := append([]string{"-A", chain}, rule...)
	return f.run(ctx, appendArgs...)
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
	name := "iptables"
	if f.useSudo() {
		// `-n` keeps the call non-interactive so a missing/expired sudo
		// credential fails fast (classified below) instead of blocking on a
		// password prompt whose output we cannot surface.
		name = "sudo"
		args = append([]string{"-n", "iptables"}, args...)
	}
	out, err := f.runner.Run(ctx, name, args...)
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
	case strings.Contains(msg, "a password is required") || strings.Contains(msg, "a terminal is required") || strings.Contains(msg, "sudo: a password"):
		return out, fmt.Errorf("%w: %s", ErrFirewallSudoFailed, strings.TrimSpace(string(out)))
	case strings.Contains(msg, "permission denied"):
		return out, fmt.Errorf("%w: iptables command failed: permission denied (run codex-dock as root or pass --sudo): %w", ErrFirewallPermissionDenied, err)
	default:
		return out, fmt.Errorf("%s %s failed: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
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

// parseManagedChainRules parses `iptables -S CODEX-DOCK` output into ordered
// rules. Only `-A CODEX-DOCK ...` lines are considered; the `-N` chain
// declaration is skipped.
func parseManagedChainRules(out []byte) []FirewallRule {
	prefix := "-A " + managedChain
	var rules []FirewallRule
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		fields := strings.Fields(line)
		rule := FirewallRule{}
		for i := 0; i < len(fields); i++ {
			switch fields[i] {
			case "-d":
				if i+1 < len(fields) {
					rule.Destination = fields[i+1]
					i++
				}
			case "-p":
				if i+1 < len(fields) {
					rule.Protocol = fields[i+1]
					i++
				}
			case "--dport":
				if i+1 < len(fields) {
					if port, err := strconv.Atoi(fields[i+1]); err == nil {
						rule.Port = port
					}
					i++
				}
			case "--comment":
				if i+1 < len(fields) {
					rule.Comment = strings.Trim(fields[i+1], `"`)
					i++
				}
			case "-j":
				if i+1 < len(fields) {
					rule.Verdict = fields[i+1]
					i++
				}
			}
		}
		if rule.Verdict == "DROP" {
			rule.Action = "block"
		} else {
			rule.Action = "allow"
		}
		rules = append(rules, rule)
	}
	return rules
}

func IsFirewallWarning(err error) bool {
	return errors.Is(err, ErrFirewallRootRequired) ||
		errors.Is(err, ErrFirewallIptablesNotFound) ||
		errors.Is(err, ErrFirewallSudoNotFound) ||
		errors.Is(err, ErrFirewallSudoFailed)
}

func normalizePorts(ports []int) []int {
	seen := map[int]struct{}{}
	normalized := make([]int, 0, len(ports))
	for _, port := range ports {
		if port <= 0 || port > 65535 {
			continue
		}
		if _, ok := seen[port]; ok {
			continue
		}
		seen[port] = struct{}{}
		normalized = append(normalized, port)
	}
	slices.Sort(normalized)
	return normalized
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

func normalizeBlockDestinations(blocks []BlockDestination) []BlockDestination {
	seen := map[string]struct{}{}
	normalized := make([]BlockDestination, 0, len(blocks))

	for _, block := range blocks {
		cidr := strings.TrimSpace(block.CIDR)
		if cidr == "" || block.Port < 0 || block.Port > 65535 {
			continue
		}
		key := cidr + "|" + strconv.Itoa(block.Port)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, BlockDestination{CIDR: cidr, Port: block.Port})
	}

	slices.SortFunc(normalized, func(a, b BlockDestination) int {
		if a.CIDR != b.CIDR {
			return strings.Compare(a.CIDR, b.CIDR)
		}
		return a.Port - b.Port
	})
	return normalized
}
