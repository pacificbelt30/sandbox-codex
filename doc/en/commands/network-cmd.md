# `codex-dock network` — Network Management

> [日本語](../../commands/network-cmd.md) | **English**
>
> [← Command Reference](../commands.md)

Manages the `dock-net` Docker network.

> Linux `iptables` firewall management is a separate command: [`codex-dock firewall`](firewall.md)

---

## `network create`

```bash
codex-dock network create [--no-internet]
```

> Automatically created by `codex-dock run` if it doesn't exist.

---

## `network rm`

```bash
codex-dock network rm
```

> Stop running containers before removing the network.

---

## `network status`

```bash
codex-dock network status
```

```
dock-net ID:     a1b2c3d4e5f6
Driver:          bridge
ICC disabled:    true
IP Masquerade:   true
Subnet:          10.200.0.0/24
```

---

## Related Documentation

- [Network Specification](../network.md)
- [`codex-dock firewall` Command](firewall.md)
- [Firewall Specification & Operations Guide](../firewall.md)
