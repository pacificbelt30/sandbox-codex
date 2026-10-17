//go:build linux

package engine

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/containerd/errdefs"
	dn "github.com/docker/docker/api/types/network"
)

// networkRecord owns a bridge, its firewall table and endpoints. The host has
// no address on an internal bridge. Its namespace has no default route or DNS.
// Only explicit proxies gain another interface; workers keep exactly one.
type networkRecord struct {
	dn.Inspect
	Slot     int
	Bridge   string
	Firewall string
}

func (b *Backend) networkDir() string { return filepath.Join(b.opts.State, "networks") }
func (b *Backend) networks() ([]*networkRecord, error) {
	dir := b.networkDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var rows []*networkRecord
	for _, f := range files {
		if !strings.HasSuffix(f.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, f.Name()))
		if err != nil {
			return nil, err
		}
		var n networkRecord
		if err = json.Unmarshal(data, &n); err != nil {
			return nil, err
		}
		if !hexID.MatchString(n.ID) || f.Name() != n.ID+".json" || n.Bridge != "cdx"+n.ID[:12] || n.Slot < 1 || n.Slot > 254 {
			return nil, fmt.Errorf("invalid network ownership record")
		}
		rows = append(rows, &n)
	}
	return rows, nil
}
func (b *Backend) network(value string) (*networkRecord, error) {
	all, err := b.networks()
	if err != nil {
		return nil, err
	}
	var found *networkRecord
	for _, n := range all {
		if n.ID == value || n.Name == value {
			if found != nil {
				return nil, fmt.Errorf("ambiguous network")
			}
			found = n
		}
	}
	if found == nil {
		return nil, fmt.Errorf("network %s: %w", value, errdefs.ErrNotFound)
	}
	return found, nil
}
func (b *Backend) saveNetwork(n *networkRecord) error {
	data, err := json.Marshal(n)
	if err != nil {
		return err
	}
	dir := b.networkDir()
	f, err := os.CreateTemp(dir, ".network-")
	if err != nil {
		return err
	}
	name := f.Name()
	defer func() { _ = os.Remove(name) }()
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(name, filepath.Join(dir, n.ID+".json")); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer func() { _ = d.Close() }()
	return d.Sync()
}
func (b *Backend) exec(ctx context.Context, tool string, args ...string) *exec.Cmd {
	c := exec.CommandContext(ctx, b.opts.Tools[tool], args...)
	c.Args[0] = tool
	return c
}
func (b *Backend) command(ctx context.Context, tool string, args ...string) error {
	c := b.exec(ctx, tool, args...)
	data, err := c.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w: %s", filepath.Base(tool), err, strings.TrimSpace(string(data)))
	}
	return nil
}
func (b *Backend) ip(ctx context.Context, args ...string) error { return b.command(ctx, "ip", args...) }
func (b *Backend) nft(ctx context.Context, script string) error {
	c := b.exec(ctx, "nft", "-f", "-")
	c.Stdin = strings.NewReader(script)
	data, err := c.CombinedOutput()
	if err != nil {
		return fmt.Errorf("owned network firewall: %w: %s", err, strings.TrimSpace(string(data)))
	}
	return nil
}
func table(n *networkRecord) string             { return "cdx_" + n.ID[:16] }
func subnet(n *networkRecord) string            { return fmt.Sprintf("10.253.%d.0/24", n.Slot) }
func address(n *networkRecord, last int) string { return fmt.Sprintf("10.253.%d.%d", n.Slot, last) }
func (b *Backend) createNetwork(ctx context.Context, name string, o dn.CreateOptions) (string, error) {
	if !o.Internal {
		data, err := os.ReadFile("/proc/sys/net/ipv4/ip_forward")
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(string(data)) != "1" {
			return "", fmt.Errorf("proxy egress requires host net.ipv4.ip_forward=1")
		}
	}
	if !validName.MatchString(name) || o.Driver != "bridge" {
		return "", fmt.Errorf("a named bridge is required")
	}
	if o.EnableIPv6 != nil && *o.EnableIPv6 {
		return "", fmt.Errorf("IPv6 is disabled for owned networks")
	}
	if _, err := b.network(name); err == nil {
		return "", fmt.Errorf("network exists: %w", errdefs.ErrAlreadyExists)
	} else if !errdefs.IsNotFound(err) {
		return "", err
	}
	all, err := b.networks()
	if err != nil {
		return "", err
	}
	slots := map[int]bool{}
	for _, n := range all {
		slots[n.Slot] = true
	}
	slot := 0
	// Refuse overlap with any connected host network, including VPN addresses.
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	var host []netip.Prefix
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			return "", err
		}
		for _, a := range addrs {
			p, e := netip.ParsePrefix(a.String())
			if e == nil {
				host = append(host, p)
			}
		}
	}
	// Connected addresses alone miss VPN/static routes through a gateway.
	routeData, err := b.exec(ctx, "ip", "-j", "route", "show", "table", "all").Output()
	if err != nil {
		return "", err
	}
	var routes []struct {
		Dst string `json:"dst"`
	}
	if err = json.Unmarshal(routeData, &routes); err != nil {
		return "", err
	}
	for _, route := range routes {
		p, err := netip.ParsePrefix(route.Dst)
		if err == nil && p.Bits() != 0 {
			host = append(host, p)
		}
	}
	for i := 1; i < 255; i++ {
		if slots[i] {
			continue
		}
		candidate := netip.MustParsePrefix(fmt.Sprintf("10.253.%d.0/24", i))
		overlap := false
		for _, p := range host {
			if p.Overlaps(candidate) {
				overlap = true
			}
		}
		if !overlap {
			slot = i
			break
		}
	}
	if slot == 0 {
		return "", fmt.Errorf("no non-overlapping network subnet available")
	}
	id, err := id()
	if err != nil {
		return "", err
	}
	n := &networkRecord{Slot: slot, Bridge: "cdx" + id[:12], Inspect: dn.Inspect{ID: id, Name: name, Driver: "bridge", Internal: o.Internal, Labels: o.Labels, Options: o.Options, Containers: map[string]dn.EndpointResource{}, IPAM: dn.IPAM{Config: []dn.IPAMConfig{{Subnet: fmt.Sprintf("10.253.%d.0/24", slot)}}}}}
	// Intent is durable before host mutation. Only this ID can retry/remove it.
	if err = b.saveNetwork(n); err != nil {
		return "", err
	}
	if err = b.materializeNetwork(ctx, n); err != nil {
		return "", err
	}
	return id, nil
}
func (b *Backend) nsName(cid string) string { return "cdx-" + cid[:24] }
func (b *Backend) nsPath(cid string) string { return "/run/netns/" + b.nsName(cid) }
func (b *Backend) newNamespace(ctx context.Context, r *record) error {
	if err := b.ip(ctx, "netns", "add", b.nsName(r.ID)); err != nil {
		return err
	}
	info, err := os.Stat(b.nsPath(r.ID))
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("cannot identify network namespace")
	}
	r.NetNSDevice = uint64(stat.Dev)
	r.NetNSInode = stat.Ino
	if err = b.save(r); err != nil {
		return err
	}
	if err := b.ip(ctx, "-n", b.nsName(r.ID), "link", "set", "lo", "up"); err != nil {
		return err
	}
	return b.command(ctx, "ip", "netns", "exec", b.nsName(r.ID), b.opts.Tools["sysctl"], "-q", "-w", "net.ipv6.conf.all.disable_ipv6=1", "net.ipv6.conf.default.disable_ipv6=1", "net.ipv4.ip_forward=0")
}
func (b *Backend) connect(ctx context.Context, value string, r *record) error {
	n, err := b.network(value)
	if err != nil {
		return err
	}
	if _, ok := r.Networks[n.Name]; ok {
		return nil
	}
	if !n.Internal && r.Config.Labels["codex-dock.managed"] == "true" {
		return fmt.Errorf("workers cannot join a proxy egress network")
	}
	if n.Internal && len(r.Networks) > 0 && r.Config.Labels["codex-dock.managed"] == "true" {
		return fmt.Errorf("workers cannot join a second network")
	}
	if n.Internal && r.Config.Labels["codex-dock.managed"] != "true" && r.Config.Labels["io.codex-dock.proxy-layout"] == "" {
		return fmt.Errorf("only workers and explicit proxies may join an internal network")
	}
	used := map[string]bool{}
	for _, e := range n.Containers {
		used[e.IPv4Address] = true
	}
	last := 2
	for ; last < 255; last++ {
		if !used[address(n, last)+"/24"] {
			break
		}
	}
	if last == 255 {
		return fmt.Errorf("network has no free endpoint")
	}
	endpoint := address(n, last)
	usedInterfaces := map[string]bool{}
	for _, endpoint := range r.Networks {
		usedInterfaces[endpoint.DriverOpts["codex-dock.interface"]] = true
	}
	ifaceIndex := 0
	for usedInterfaces["eth"+strconv.Itoa(ifaceIndex)] {
		ifaceIndex++
	}
	ifname := "eth" + strconv.Itoa(ifaceIndex)
	hostVeth := "v" + r.ID[:6] + n.ID[:6]

	// Record before creating the interface; interrupted attachment remains
	// visible and removable by immutable IDs instead of adopting foreign links.
	r.Networks[n.Name] = &dn.EndpointSettings{NetworkID: n.ID, EndpointID: hostVeth, IPAddress: endpoint, IPPrefixLen: 24, Gateway: n.IPAM.Config[0].Gateway, DriverOpts: map[string]string{"codex-dock.interface": ifname}}
	n.Containers[r.ID] = dn.EndpointResource{Name: r.Name, EndpointID: hostVeth, IPv4Address: endpoint + "/24"}
	if err = b.saveNetwork(n); err != nil {
		return err
	}
	if err = b.save(r); err != nil {
		return err
	}
	return b.materializeEndpoint(ctx, n, r, r.Networks[n.Name])
}
func (b *Backend) materializeEndpoint(ctx context.Context, n *networkRecord, r *record, e *dn.EndpointSettings) error {
	hostVeth := e.EndpointID
	peer := "p" + r.ID[:6] + n.ID[:6]
	ifname := e.DriverOpts["codex-dock.interface"]
	endpoint := e.IPAddress
	if hostVeth != "v"+r.ID[:6]+n.ID[:6] || !regexp.MustCompile(`^eth[0-9]+$`).MatchString(ifname) {
		return fmt.Errorf("invalid endpoint identity")
	}
	var err error
	if err = b.ip(ctx, "link", "add", hostVeth, "type", "veth", "peer", "name", peer); err != nil {
		return err
	}
	if err = b.ip(ctx, "link", "set", hostVeth, "master", n.Bridge); err != nil {
		return err
	}
	if err = b.ip(ctx, "link", "set", hostVeth, "alias", "codex-dock:"+r.ID+":"+n.ID); err != nil {
		return err
	}
	if err = b.ip(ctx, "link", "set", peer, "netns", b.nsName(r.ID)); err != nil {
		return err
	}
	if err = b.ip(ctx, "-n", b.nsName(r.ID), "link", "set", peer, "name", ifname); err != nil {
		return err
	}
	if err = b.ip(ctx, "-n", b.nsName(r.ID), "addr", "add", endpoint+"/24", "dev", ifname); err != nil {
		return err
	}
	if err = b.ip(ctx, "-n", b.nsName(r.ID), "link", "set", ifname, "up"); err != nil {
		return err
	}
	if err = b.ip(ctx, "link", "set", hostVeth, "up"); err != nil {
		return err
	}
	if !n.Internal {
		return b.ip(ctx, "-n", b.nsName(r.ID), "route", "add", "default", "via", address(n, 1))
	}
	return nil
}
func (b *Backend) disconnect(ctx context.Context, n *networkRecord, cid string) error {
	e, ok := n.Containers[cid]
	if !ok {
		return nil
	}
	if !hexID.MatchString(cid) || e.EndpointID != "v"+cid[:6]+n.ID[:6] {
		return fmt.Errorf("endpoint ownership mismatch")
	}
	if err := b.deleteEndpointLink(ctx, n, cid, e.EndpointID); err != nil {
		return err
	}
	delete(n.Containers, cid)
	if err := b.saveNetwork(n); err != nil {
		return err
	}
	r, _, err := b.load(ctx, cid)
	if errdefs.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	delete(r.Networks, n.Name)
	return b.save(r)
}
func (b *Backend) removeNamespace(ctx context.Context, r *record) error {
	all, err := b.networks()
	if err != nil {
		return err
	}
	for _, n := range all {
		if err = b.disconnect(ctx, n, r.ID); err != nil {
			return err
		}
	}
	if _, err = os.Stat(b.nsPath(r.ID)); os.IsNotExist(err) {
		return nil
	}
	return b.dropNamespace(ctx, r)
}
func (b *Backend) removeNetwork(ctx context.Context, n *networkRecord) error {
	if len(n.Containers) > 0 {
		return fmt.Errorf("network still has attached endpoints")
	}
	if data, err := os.ReadFile("/sys/class/net/" + n.Bridge + "/ifalias"); err == nil {
		if strings.TrimSpace(string(data)) != "codex-dock:"+n.ID {
			return fmt.Errorf("refusing to remove foreign bridge")
		}
		if err = b.ip(ctx, "link", "del", n.Bridge); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	c := b.exec(ctx, "nft", "list", "table", "inet", table(n))
	if err := c.Run(); err == nil {
		if err = b.nft(ctx, "delete table inet "+table(n)+"\n"); err != nil {
			return err
		}
	}
	return os.Remove(filepath.Join(b.networkDir(), n.ID+".json"))
}

func (b *Backend) resolverFiles(r *record) error {
	hosts := "127.0.0.1 localhost\n"
	for _, h := range r.Host.ExtraHosts {
		name, addr, ok := strings.Cut(h, ":")
		if !ok || strings.ContainsAny(name, " \t\r\n") {
			return fmt.Errorf("invalid static hostname")
		}
		if _, err := netip.ParseAddr(addr); err != nil {
			return err
		}
		hosts += addr + " " + name + "\n"
	}
	// Preserve name-based proxy access in explicit runc compatibility too.
	// These aliases come only from this container's owned networks.
	known := map[string]bool{}
	for _, entry := range r.Host.ExtraHosts {
		name, _, _ := strings.Cut(entry, ":")
		known[name] = true
	}
	for name := range r.Networks {
		n, err := b.network(name)
		if err != nil {
			return err
		}
		for _, peer := range n.Containers {
			if validName.MatchString(peer.Name) && !known[peer.Name] {
				prefix, err := netip.ParsePrefix(peer.IPv4Address)
				if err != nil {
					return err
				}
				hosts += prefix.Addr().String() + " " + peer.Name + "\n"
				known[peer.Name] = true
			}
		}
	}
	resolv := ""
	dns := r.Host.DNS
	if len(dns) == 0 {
		dns = b.opts.DNS
	}
	for _, server := range dns {
		if _, err := netip.ParseAddr(server); err != nil {
			return err
		}
		resolv += "nameserver " + server + "\n"
	}
	if resolv == "" {
		data, err := os.ReadFile("/run/systemd/resolve/resolv.conf")
		if err != nil {
			data, err = os.ReadFile("/etc/resolv.conf")
		}
		if err != nil {
			return err
		}
		resolv = string(data)
	}
	for file, data := range map[string]string{"hosts": hosts, "hostname": r.Name + "\n", "resolv.conf": resolv} {
		if err := os.WriteFile(filepath.Join(b.path(r.ID), file), []byte(data), 0644); err != nil {
			return err
		}
	}
	return nil
}

func (b *Backend) networkHTTP(w http.ResponseWriter, r *http.Request, ctx context.Context, path string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if path == "/networks/create" {
		var req struct {
			Name string
			dn.CreateOptions
		}
		if err := decode(r, &req); err != nil {
			failure(w, err)
			return
		}
		id, err := b.createNetwork(ctx, req.Name, req.CreateOptions)
		if err != nil {
			failure(w, err)
			return
		}
		reply(w, dn.CreateResponse{ID: id})
		return
	}
	if path == "/networks" {
		all, err := b.networks()
		if err != nil {
			failure(w, err)
			return
		}
		rows := []dn.Inspect{}
		for _, n := range all {
			rows = append(rows, n.Inspect)
		}
		reply(w, rows)
		return
	}
	parts := strings.Split(strings.TrimPrefix(path, "/networks/"), "/")
	n, err := b.network(parts[0])
	if err != nil {
		failure(w, err)
		return
	}
	if len(parts) == 1 {
		if r.Method == "DELETE" {
			err = b.removeNetwork(ctx, n)
		} else {
			if err = b.verifyNetwork(ctx, n); err != nil {
				failure(w, err)
				return
			}
			reply(w, n.Inspect)
			return
		}
	} else {
		var req struct{ Container string }
		err = decode(r, &req)
		if err == nil {
			var rec *record
			rec, _, err = b.load(ctx, req.Container)
			if err == nil {
				switch parts[1] {
				case "connect":
					err = b.connect(ctx, n.ID, rec)
				case "disconnect":
					err = b.disconnect(ctx, n, rec.ID)
				default:
					err = fmt.Errorf("unknown network operation")
				}
			}
		}
	}
	if err != nil {
		failure(w, err)
		return
	}
	w.WriteHeader(200)
}

func (b *Backend) closePorts(cid string) {
	for _, l := range b.ports[cid] {
		_ = l.Close()
	}
	delete(b.ports, cid)
}
func (b *Backend) openPorts(ctx context.Context, r *record) error {
	b.closePorts(r.ID)
	for port, bindings := range r.Host.PortBindings {
		if port.Proto() != "tcp" {
			return fmt.Errorf("only loopback TCP publication is supported")
		}
		for bindingIndex, binding := range bindings {
			if binding.HostIP != "127.0.0.1" {
				return fmt.Errorf("only loopback ports may be published")
			}
			var target string
			for name, e := range r.Networks {
				n, err := b.network(name)
				if err != nil {
					return err
				}
				if !n.Internal {
					target = net.JoinHostPort(e.IPAddress, port.Port())
				}
			}
			if target == "" {
				return fmt.Errorf("published proxy requires an egress endpoint")
			}
			listener, err := net.Listen("tcp", net.JoinHostPort(binding.HostIP, binding.HostPort))
			if err != nil {
				return err
			}
			binding.HostPort = strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
			r.Host.PortBindings[port][bindingIndex] = binding
			if err = b.save(r); err != nil {
				_ = listener.Close()
				return err
			}
			b.ports[r.ID] = append(b.ports[r.ID], listener)
			go relay(listener, target)
		}
	}
	return nil
}
func relay(listener net.Listener, target string) {
	for {
		c, err := listener.Accept()
		if err != nil {
			return
		}
		go func() {
			defer func() { _ = c.Close() }()
			up, err := net.DialTimeout("tcp", target, 5*time.Second)
			if err != nil {
				return
			}
			defer func() { _ = up.Close() }()
			done := make(chan struct{})
			go func() {
				_, _ = io.Copy(up, c)
				if t, ok := up.(*net.TCPConn); ok {
					_ = t.CloseWrite()
				}
				close(done)
			}()
			_, _ = io.Copy(c, up)
			if t, ok := c.(*net.TCPConn); ok {
				_ = t.CloseWrite()
			}
			_ = c.SetReadDeadline(time.Now().Add(time.Second))
			<-done
		}()
	}
}

// Dragonball 4.2 can leave tcfilter ingress state behind. Regenerate only the
// stopped worker's namespace and veth, preserving its bridge, IP and proxies.
// Proxy namespaces are kept intact because they serve other workers as well.
func (b *Backend) refreshWorkerNamespace(ctx context.Context, r *record) error {
	if r.Config.Labels["codex-dock.managed"] != "true" || len(r.Networks) != 1 {
		return fmt.Errorf("Dragonball requires one managed worker endpoint")
	}
	for name, e := range r.Networks {
		n, err := b.network(name)
		if err != nil {
			return err
		}
		if !n.Internal || n.Containers[r.ID].EndpointID != e.EndpointID {
			return fmt.Errorf("worker endpoint ownership mismatch")
		}
		if err = b.deleteEndpointLink(ctx, n, r.ID, e.EndpointID); err != nil {
			return err
		}
	}
	if err := b.dropNamespace(ctx, r); err != nil {
		return err
	}
	if err := b.newNamespace(ctx, r); err != nil {
		return err
	}
	for name, e := range r.Networks {
		n, err := b.network(name)
		if err != nil {
			return err
		}
		if err = b.materializeEndpoint(ctx, n, r, e); err != nil {
			return err
		}
	}
	return nil
}
func (b *Backend) dropNamespace(ctx context.Context, r *record) error {
	info, err := os.Stat(b.nsPath(r.ID))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	s, ok := info.Sys().(*syscall.Stat_t)
	if !ok || uint64(s.Dev) != r.NetNSDevice || s.Ino != r.NetNSInode {
		return fmt.Errorf("network namespace ownership changed")
	}
	return b.ip(ctx, "netns", "del", b.nsName(r.ID))
}
func (b *Backend) deleteEndpointLink(ctx context.Context, n *networkRecord, cid, endpoint string) error {
	if !hexID.MatchString(cid) || endpoint != "v"+cid[:6]+n.ID[:6] {
		return fmt.Errorf("invalid endpoint ownership")
	}
	data, err := os.ReadFile("/sys/class/net/" + endpoint + "/ifalias")
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(data)) != "codex-dock:"+cid+":"+n.ID {
		return fmt.Errorf("refusing to remove foreign interface")
	}
	return b.ip(ctx, "link", "del", endpoint)
}

func (b *Backend) materializeNetwork(ctx context.Context, n *networkRecord) error {
	var err error
	if err = b.ip(ctx, "link", "add", n.Bridge, "type", "bridge"); err != nil {
		return err
	}
	if err = b.ip(ctx, "link", "set", n.Bridge, "alias", "codex-dock:"+n.ID); err != nil {
		return err
	}
	if err = b.ip(ctx, "link", "set", n.Bridge, "up"); err != nil {
		return err
	}
	nameTable := table(n)
	script := fmt.Sprintf("add table inet %s\nadd chain inet %s input { type filter hook input priority -10; policy accept; }\nadd chain inet %s forward { type filter hook forward priority -10; policy accept; }\n", nameTable, nameTable, nameTable)
	if n.Internal {
		script += fmt.Sprintf("add rule inet %s input iifname %q drop\nadd rule inet %s forward iifname %q oifname %q accept\nadd rule inet %s forward iifname %q drop\nadd rule inet %s forward oifname %q drop\n", nameTable, n.Bridge, nameTable, n.Bridge, n.Bridge, nameTable, n.Bridge, nameTable, n.Bridge)
	} else {
		if err = b.ip(ctx, "addr", "add", address(n, 1)+"/24", "dev", n.Bridge); err != nil {
			return err
		}
		n.IPAM.Config[0].Gateway = address(n, 1)
		// Proxy egress is NATed. No worker is allowed on this network.
		script += fmt.Sprintf("add chain inet %s postrouting { type nat hook postrouting priority srcnat; policy accept; }\nadd rule inet %s postrouting ip saddr %s oifname != %q masquerade\n", nameTable, nameTable, subnet(n), n.Bridge)
	}
	if err = b.nft(ctx, script); err != nil {
		return err
	}
	var errHash error
	n.Firewall, errHash = b.firewallHash(ctx, n)
	if errHash != nil {
		return errHash
	}
	if err = b.saveNetwork(n); err != nil {
		return err
	}
	return nil
}
func (b *Backend) firewallHash(ctx context.Context, n *networkRecord) (string, error) {
	data, err := b.exec(ctx, "nft", "-j", "list", "table", "inet", table(n)).Output()
	if err != nil {
		return "", fmt.Errorf("owned network firewall missing: %w", err)
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum), nil
}
func (b *Backend) verifyNetwork(ctx context.Context, n *networkRecord) error {
	data, err := os.ReadFile("/sys/class/net/" + n.Bridge + "/ifalias")
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(data)) != "codex-dock:"+n.ID {
		return fmt.Errorf("bridge ownership changed")
	}
	actual, err := b.firewallHash(ctx, n)
	if err != nil {
		return err
	}
	if n.Firewall == "" || n.Firewall != actual {
		return fmt.Errorf("owned network firewall was modified")
	}
	iface, err := net.InterfaceByName(n.Bridge)
	if err != nil {
		return err
	}
	addresses, err := iface.Addrs()
	if err != nil {
		return err
	}
	if n.Internal {
		for _, a := range addresses {
			p, err := netip.ParsePrefix(a.String())
			if err == nil && p.Addr().Is4() {
				return fmt.Errorf("internal bridge acquired a host IPv4 address")
			}
		}
	}
	peers, err := os.ReadDir("/sys/class/net/" + n.Bridge + "/brif")
	if err != nil {
		return err
	}
	allowed := map[string]bool{}
	for _, e := range n.Containers {
		allowed[e.EndpointID] = true
	}
	for _, p := range peers {
		if !allowed[p.Name()] {
			return fmt.Errorf("untracked peer on owned bridge")
		}
	}
	return nil
}
