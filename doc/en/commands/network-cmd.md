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
> On Linux this also installs `iptables` rules, so root privileges are required.

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
