# Firewall Specification & Operations Guide

> [日本語](../firewall.md) | **English**

`codex-dock firewall` is a dedicated command for Linux hosts that controls `iptables` via `DOCKER-USER` to restrict unwanted destinations from `dock-net`.

---

## Scope of This Page

- `network` is for creating Docker networks.
- `firewall` is for allowing/denying traffic.
- Recommended order in practice: **`network create` → `firewall create`**.

---

## Supported Environment & Prerequisites

`firewall` is effective when all of the following are true:

- Linux host
- Run as root (or pass `--sudo`)
- `iptables` command is available

If prerequisites are missing, codex-dock prints warnings and continues.

### `--sudo` when not root

When you cannot run as root, add `--sudo` to `run` / `firewall create` /
`firewall rm` / `network rm` to run **only the `iptables` calls** via `sudo`
(codex-dock itself keeps running as the unprivileged user, so config and
credential path resolution are unaffected).

- On an interactive terminal it prompts for a password once (internally `sudo -v`).
- In a non-interactive environment (CI / TUI / `--detach`) it never blocks on a
  prompt: it relies on cached credentials or a `NOPASSWD` sudoers entry, and
  fails explicitly on the first `iptables` call if neither is available.
- The config key `firewall.sudo = true` makes this the default (CLI `--sudo` wins).

```bash
# Apply without root via --sudo (prompts for a password only on the iptables calls)
codex-dock firewall create --sudo
codex-dock run --agent claude --sudo
```

---

## Rule Application Model

`codex-dock firewall create` builds the `CODEX-DOCK` chain to be evaluated
**top to bottom** with this model:

1. Add jump path from `DOCKER-USER` to `CODEX-DOCK`
2. **Allow (RETURN)**: Auth Proxy + any `--allow-host` destinations
3. **Drop (DROP)**: private/link-local + any `--block-host` destinations
4. End with `RETURN` to hand control back to other Docker rules

Because allow rules are evaluated first, **`--allow-host` takes precedence over
`--block-host`**. The chain ends with `RETURN` (so public internet outside the
private ranges passes through), which is why blocking a public IP requires an
explicit `--block-host`.

### Allowed Traffic

- Auth Proxy `IP:PORT` resolved from `--proxy-container-url`
- Traffic from `dock-net` subnet (`10.200.0.0/24`) to the same proxy port
- Destinations added via `--allow-host IP:PORT` (or config `firewall.allow_hosts`)

### Dropped Traffic

- private/link-local ranges: `10/8`, `172.16/12`, `192.168/16`, `169.254/16`, `127/8`
- Destinations added via `--block-host CIDR|IP|IP:PORT` (or config `firewall.block_hosts`), IPv4

### Customizing allow / block

```bash
# Allow an internal registry (203.0.113.10:5000), block a whole external range
sudo codex-dock firewall create \
  --allow-host 203.0.113.10:5000 \
  --block-host 203.0.113.0/24

# The same flags work on `run`
codex-dock run --agent claude --allow-host 203.0.113.10:5000 --block-host 198.51.100.9:443
```

To avoid repeating flags, persist them in `~/.config/codex-dock/config.toml`:

```toml
[firewall]
proxy_container_url = "http://codex-auth-proxy:18080"
allow_hosts = ["203.0.113.10:5000"]   # always allowed
block_hosts = ["203.0.113.0/24"]      # always blocked
```

> `--block-host` accepts IPv4 `CIDR` / `IP` / `IP:PORT`. A bare `IP` becomes
> `/32`; `IP:PORT` drops only that TCP port. Check the applied rules with
> `codex-dock firewall status` (the `ALLOW` / `BLOCK` listing; `--block-host`
> entries are labelled `custom block`).

---

## Recommended Operations Flow

```bash
# 1) Create network
codex-dock network create

# 2) Apply firewall rules (root)
sudo codex-dock firewall create --proxy-container-url http://codex-auth-proxy:18080

# 3) Verify state
sudo codex-dock firewall status
```

If `dock-net` / `dock-net-proxy` are missing, `firewall create` prompts:
`Create <network> now? [y/N]:`.

---

## Troubleshooting

### Cannot reach Auth Proxy

- Check rule order with `iptables -S DOCKER-USER`
- Ensure no earlier DROP rule blocks proxy allow rules
- Verify `--proxy-container-url` hostname/port

### Firewall behavior is not available on non-Linux

- macOS / Windows (Docker Desktop) are outside Linux `iptables` automation scope

---

## Related Documentation

- [`codex-dock firewall` Command Reference](commands/firewall.md)
- [Network Specification (dock-net)](network.md)
- [Using Auth Proxy Standalone](proxy-standalone.md)
