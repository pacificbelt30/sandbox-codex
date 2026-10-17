package cmd

import (
	"context"
	"fmt"
	"os/signal"
	"os/user"
	"strconv"
	"syscall"

	"github.com/pacificbelt30/codex-dock/internal/kataguard"
	"github.com/spf13/cobra"
)

func init() {
	var cfg kataguard.Config
	var group string
	node := &cobra.Command{Use: "node", Short: "Manage the trusted local Kata verification service"}
	serve := &cobra.Command{Use: "serve", Short: "Serve scoped Kata verification over a root-owned Unix socket", RunE: func(cmd *cobra.Command, args []string) error {
		entry, err := user.LookupGroup(group)
		if err != nil {
			return fmt.Errorf("node operator group: %w", err)
		}
		gid, err := strconv.Atoi(entry.Gid)
		if err != nil {
			return err
		}
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		cfg.Endpoint = "unix://" + cfg.Socket
		return kataguard.Serve(ctx, cfg, cfg.Socket, gid)
	}}
	f := serve.Flags()
	f.StringVar(&cfg.Manifest, "manifest", "/etc/codex-dock/kata-assets.json", "Pinned root-owned Kata assets")
	f.StringVar(&cfg.CgroupRoot, "cgroup-root", "/sys/fs/cgroup/codex-dock", "Dedicated empty/delegated cgroup v2 subtree")
	f.StringVar(&cfg.StateDir, "state-dir", "/var/lib/codex-dock-node", "Private node admission state")
	f.StringVar(&cfg.Socket, "socket", "/run/codex-dock-node.sock", "Root-owned management socket (never worker-mounted)")
	f.StringVar(&group, "group", "codex-dock", "Trusted host operator group allowed to use the socket")
	f.StringSliceVar(&cfg.DNS, "proxy-dns", nil, "Reachable non-loopback DNS servers for proxy namespaces")
	f.IntVar(&cfg.MaxWorkers, "max-workers", 2, "Maximum simultaneously allocated worker envelopes")
	node.AddCommand(serve)
	rootCmd.AddCommand(node)
}
