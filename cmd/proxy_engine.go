package cmd

import (
	"context"
	"fmt"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	dn "github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
	"github.com/pacificbelt30/codex-dock/internal/engine"
	"strconv"
	"strings"
)

// The proxy spec has one source of truth for credentials, policy, and roles.
// Convert its structured option vector to the node DTO; no shell/CLI is run.
func proxyConfiguration(a proxyRunArgs) (*container.Config, *container.HostConfig, error) {
	args := buildProxyRunArgs(a)
	c := &container.Config{User: "0:0", Labels: map[string]string{}}
	h := &container.HostConfig{Runtime: "runc", CapDrop: []string{"ALL"}, SecurityOpt: []string{"no-new-privileges:true"}, PortBindings: nat.PortMap{}}
	for i := 1; i < len(args); i++ {
		arg := args[i]
		if arg == "-d" {
			continue
		}
		if arg == a.image {
			c.Image = a.image
			c.Cmd = append([]string{}, args[i+1:]...)
			return c, h, nil
		}
		if i+1 >= len(args) {
			return nil, nil, fmt.Errorf("invalid proxy spec")
		}
		i++
		value := args[i]
		switch arg {
		case "--name":
		case "--label":
			k, v, ok := strings.Cut(value, "=")
			if !ok {
				return nil, nil, fmt.Errorf("invalid proxy label")
			}
			c.Labels[k] = v
		case "--network":
			h.NetworkMode = container.NetworkMode(value)
		case "-e":
			c.Env = append(c.Env, value)
		case "-v":
			parts := strings.Split(value, ":")
			if len(parts) != 3 || parts[2] != "ro" {
				return nil, nil, fmt.Errorf("proxy credentials require read-only bind mounts")
			}
			h.Mounts = append(h.Mounts, mount.Mount{Type: mount.TypeBind, Source: parts[0], Target: parts[1], ReadOnly: true})
		case "-p":
			parts := strings.Split(value, ":")
			if len(parts) != 3 || parts[0] != "127.0.0.1" {
				return nil, nil, fmt.Errorf("proxy admin must publish on loopback")
			}
			if _, err := strconv.Atoi(parts[1]); err != nil {
				return nil, nil, err
			}
			p, err := nat.NewPort("tcp", parts[2])
			if err != nil {
				return nil, nil, err
			}
			h.PortBindings[p] = []nat.PortBinding{{HostIP: parts[0], HostPort: parts[1]}}
		default:
			return nil, nil, fmt.Errorf("unsupported proxy option %q", arg)
		}
	}
	return nil, nil, fmt.Errorf("proxy spec has no image")
}
func stopProxy(ctx context.Context, name string) error {
	c, err := engine.NewClient()
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	return c.ContainerStop(ctx, name, container.StopOptions{})
}
func removeProxy(ctx context.Context, name string, force bool) error {
	c, err := engine.NewClient()
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	return c.ContainerRemove(ctx, name, container.RemoveOptions{Force: force})
}
func ensureNodeBridge(ctx context.Context, name string) error {
	c, err := engine.NewClient()
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	_, err = c.NetworkInspect(ctx, name, dn.InspectOptions{})
	if err == nil {
		return nil
	}
	_, err = c.NetworkCreate(ctx, name, dn.CreateOptions{Driver: "bridge", Labels: map[string]string{"codex-dock.managed": "true"}})
	return err
}
