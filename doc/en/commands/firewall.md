# `codex-dock firewall` — Removed

> [日本語](../../commands/firewall.md) | **English**
>
> [← Command Reference](../commands.md)

This command has been removed. Network isolation is provided by Docker-native primitives (per-worker `Internal` networks + the proxy router); no `iptables`/`sudo` required.

- Overview: [Network Specification](../network.md)
- Restrict egress by domain: `codex-dock proxy run --forward-allow-domain <domain>`
