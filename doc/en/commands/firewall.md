# `codex-dock firewall` — Firewall Management

> [日本語](../../commands/firewall.md) | **English**
>
> [← Command Reference](../commands.md)

`codex-dock firewall` manages Linux `iptables` rules for `dock-net`.
Its role is separate from `codex-dock network` (network provisioning), so treat them as separate operational steps.

---

## Preflight Checks

- Linux host
- Root privileges
- `iptables` installed

If checks fail, codex-dock shows warnings and continues.

---

## `firewall create`

```bash
codex-dock firewall create [--no-internet] [--proxy-container-url URL]
```

| Option | Default | Description |
|---|---|---|
| `--no-internet` | `false` | Disable IP Masquerade when creating `dock-net` |
| `--proxy-container-url` | `http://codex-auth-proxy:18080` | Auth Proxy URL to allow |

### Behavior Summary

1. Add jump path from `DOCKER-USER` to `CODEX-DOCK`
2. Insert Auth Proxy allow rules first
3. Drop private/link-local destinations
4. End with `RETURN`

If `dock-net` / `dock-net-proxy` are missing, the command prompts whether to create them.

---

## `firewall status`

```bash
codex-dock firewall status
```

Reports:

- Linux support
- Root execution status
- `iptables` detection
- `CODEX-DOCK` chain existence
- `DOCKER-USER -> CODEX-DOCK` jump rule existence
- `DOCKER-USER` default policy
- Final jump verdict in `CODEX-DOCK`

---

## `firewall rm`

```bash
codex-dock firewall rm
```

Removes the `DOCKER-USER -> CODEX-DOCK` jump rule and deletes the `CODEX-DOCK` chain.

---

## Related Documentation

- [Firewall Specification & Operations Guide](../firewall.md)
- [`codex-dock network` Command](network-cmd.md)
