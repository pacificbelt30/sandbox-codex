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
- Run as root
- `iptables` command is available

If prerequisites are missing, codex-dock prints warnings and continues.

---

## Rule Application Model

`codex-dock firewall create` applies rules with this model:

1. Add jump path from `DOCKER-USER` to `CODEX-DOCK`
2. Explicitly allow Auth Proxy traffic first
3. Drop private/link-local destinations
4. End with `RETURN` to hand control back to other Docker rules

### Allowed Traffic

- Auth Proxy `IP:PORT` resolved from `--proxy-container-url`
- Traffic from `dock-net` subnet (`10.200.0.0/24`) to the same proxy port

### Dropped Traffic

- private/link-local ranges: `10/8`, `172.16/12`, `192.168/16`, `169.254/16`, `127/8`

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
