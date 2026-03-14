# Network Specification (dock-net)

> [日本語](../network.md) | **English**

codex-dock uses a dedicated Docker bridge network **dock-net** to isolate worker containers.

---

## dock-net Base Configuration

| Item | Value |
|---|---|
| Network name | `dock-net` |
| Driver | `bridge` |
| Bridge name | `dock-net0` |
| Subnet | `10.200.0.0/24` |
| Gateway | `10.200.0.1` |
| ICC | `false` (inter-container communication disabled) |
| IP Masquerade | `true` by default (`false` with `--no-internet`) |

---

## Network Management Commands

### `codex-dock network create`

```bash
codex-dock network create [--no-internet]
```

- Creates `dock-net`.
- `--no-internet` disables IP Masquerade to block outbound internet access.
- `codex-dock run` auto-creates `dock-net` when missing.

### `codex-dock network status`

```bash
codex-dock network status
```

Shows current `dock-net` state (driver / subnet / ICC / IP Masquerade).

### `codex-dock network rm`

```bash
codex-dock network rm
```

Removes `dock-net`. Stop running containers that use it first.

---

## Relationship with Firewall

- `network create` only creates Docker networks.
- Linux `iptables` traffic control (`codex-dock firewall`) is a separate feature.
- In real environments, configuring firewall after network creation is recommended.

See the dedicated firewall docs for details:

- [Firewall Specification & Operations Guide](firewall.md)
- [`codex-dock firewall` Command Reference](commands/firewall.md)

---

## Notes

- macOS / Windows (Docker Desktop) do not provide equivalent automatic Linux `iptables` control.
