# Firewall (Removed / Replaced by the Router Model)

> [日本語](../firewall.md) | **English**

> **This command has been removed.** The old `codex-dock firewall` (Linux `iptables` `DOCKER-USER` / `CODEX-DOCK` chain control) and the related `--allow-host` / `--block-host` / `--no-firewall` / `--sudo` flags are gone.

## What changed

Network isolation is now enforced using **Docker-native primitives only** — no `iptables`, no `sudo`.

- Each worker is isolated on its own `Internal` network (`dock-net-w-<name>`); workers cannot reach each other (separate L2 segments).
- `Internal: true` means workers cannot reach the host or the internet directly.
- The proxy container acts as the router and is the only egress path (HTTP CONNECT forward proxy + API reverse routes).

See [Network Specification](network.md) for details.

## Mapping from the old feature

| Old (iptables) | New (Docker-native) |
|---|---|
| `firewall create` drops private/link-local | `Internal` networks block host/private reachability automatically |
| `--allow-host` to add allowed destinations | Not needed (egress is proxy-only) |
| `--block-host` to add blocked destinations | `proxy run --forward-allow-domain` for a domain allowlist |
| `--sudo` to run as root | Not needed (the Docker daemon handles it) |
