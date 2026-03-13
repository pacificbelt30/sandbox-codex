# `codex-dock network` — Network Management

> [日本語](../../commands/network-cmd.md) | **English**
>
> [← Command Reference](../commands.md)

Manages the `dock-net` Docker network.
For dock-net specifications and security policy, see [Network Specification](../network.md).

---

## `network create`

```bash
codex-dock network create [--no-internet]
```

> Automatically created by `codex-dock run` if it doesn't exist.

---

## `firewall create`

```bash
codex-dock firewall create [--no-internet]
```

> Applies Linux `iptables` rules for dock-net.
> If root privileges are missing or `iptables` is unavailable, a warning is shown and execution continues.

## `firewall status`

```bash
codex-dock firewall status
```

Shows dock-net firewall state (Linux support, root execution, iptables presence, chain and jump rule presence).

## `firewall rm`

```bash
codex-dock firewall rm
```

Removes dock-net firewall rules.
If root privileges are missing or `iptables` is unavailable, a warning is shown and execution continues.

---

## `network rm`

```bash
codex-dock network rm
```

> Stop all running containers before removing the network.

---

## `network status`

```bash
codex-dock network status
```

```
dock-net status:
  ID:            a1b2c3d4e5f6789012345678
  Driver:        bridge
  Subnet:        10.200.0.0/24
  ICC:           disabled
  IP Masquerade: enabled
```

---

## Related Documentation

- [Network Specification](../network.md) — dock-net configuration, security policy, and troubleshooting
