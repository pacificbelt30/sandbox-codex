# `codex-dock firewall` — Firewall Management

> [日本語](../../commands/firewall.md) | **English**
>
> [← Command Reference](../commands.md)

`codex-dock firewall` manages Linux `iptables` rules for `dock-net`.
Its role is separate from `codex-dock network` (network provisioning), so treat them as separate operational steps.

---

## Preflight Checks

- Linux host
- Root privileges (or pass `--sudo`)
- `iptables` installed

If checks fail, codex-dock shows warnings and continues.

> When not running as root, pass `--sudo` to run only the `iptables` calls via
> `sudo`. On an interactive terminal it prompts for a password once; in a
> non-interactive environment (CI / TUI / `--detach`) it relies on cached
> credentials or a NOPASSWD sudoers entry and never blocks on a prompt.

---

## `firewall create`

```bash
codex-dock firewall create [--no-internet] [--proxy-container-url URL] [--allow-host IP:PORT ...] [--block-host CIDR ...] [--sudo]
```

| Option | Default | Description |
|---|---|---|
| `--no-internet` | `false` | Disable IP Masquerade when creating `dock-net` |
| `--proxy-container-url` | `http://codex-auth-proxy:18080` | Auth Proxy URL to allow |
| `--allow-host` | (none) | Extra `IP:PORT` destination to allow. Repeatable. Must be an IP literal, not a hostname (IPv6 as `[::1]:PORT`) |
| `--block-host` | (none) | Extra `CIDR` / `IP` / `IP:PORT` destination to block (IPv4). Repeatable. `--allow-host` takes precedence |
| `--sudo` | `false` | When not root, run only the `iptables` calls via `sudo`. Prompts once on an interactive terminal; uses NOPASSWD/cached credentials when non-interactive |

```bash
# Example: allow an internal registry (203.0.113.10:5000) while creating the firewall
sudo codex-dock firewall create --allow-host 203.0.113.10:5000

# Example: block a specific range/host
sudo codex-dock firewall create --block-host 203.0.113.0/24 --block-host 198.51.100.9:443

# Example: apply without root via --sudo (prompts for a password only on the iptables calls)
codex-dock firewall create --sudo --block-host 203.0.113.0/24

# Can also be supplied directly on run
codex-dock run --agent claude --allow-host 203.0.113.10:5000 --block-host 203.0.113.0/24
```

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

Starts with a one-line `Firewall: Active / Not active / Unavailable` verdict, and
when the firewall is not active it suggests the next command to run
(e.g. `sudo codex-dock firewall create`). It then reports:

- Linux support
- Root execution status
- `iptables` detection
- `CODEX-DOCK` chain existence
- `DOCKER-USER -> CODEX-DOCK` jump rule existence
- `DOCKER-USER` default policy
- Final jump verdict in `CODEX-DOCK`

Finally it lists the **allow/block rules** of the `CODEX-DOCK` chain in evaluation
order, so you can see at a glance which destinations are `ALLOW`ed or `BLOCK`ed —
including any extra destinations added with `--allow-host`.

```text
Rules (CODEX-DOCK chain, evaluated top to bottom):
  ALLOW  172.17.0.1/32       tcp/18080  auth proxy / allowed host
  ALLOW  203.0.113.10/32     tcp/5000   auth proxy / allowed host
  ALLOW  10.200.0.0/24       tcp/18080  dock-net subnet -> proxy
  BLOCK  10.0.0.0/8          all        private/link-local
  BLOCK  172.16.0.0/12       all        private/link-local
  BLOCK  192.168.0.0/16      all        private/link-local
  BLOCK  169.254.0.0/16      all        private/link-local
  BLOCK  127.0.0.0/8         all        private/link-local
  ALLOW  any                 all        default: hand back to Docker rules
```

---

## `firewall rm`

```bash
codex-dock firewall rm [--sudo]
```

Removes the `DOCKER-USER -> CODEX-DOCK` jump rule and deletes the `CODEX-DOCK` chain.
Removal also requires root because it touches `iptables`; pass `--sudo` when not running as root.

---

## Related Documentation

- [Firewall Specification & Operations Guide](../firewall.md)
- [`codex-dock network` Command](network-cmd.md)
