# Network Specification (Proxy Router + Per-Worker Networks)

> [日本語](../network.md) | **English**

codex-dock enforces network isolation using **Docker-native primitives only** — no `iptables`, no `sudo`. The isolation rules are managed by the Docker daemon (already root).

---

## Topology

```
                         ┌──────────────── internet (NAT/masquerade ON)
          dock-net-proxy ─┤  ← proxy's egress bridge
                          │
                  ┌───────┴────────┐
                  │ codex-auth-    │  multi-homed onto each worker network
                  │ proxy (router) │  data-plane :18080 / admin :18081
                  └─┬───────────┬──┘
   Internal          │           │          Internal
 dock-net-w-A ───────┘           └───────── dock-net-w-B
   (no NAT / no host route)             (separate L2 segment)
       │                                  │
   worker A                            worker B   ← cannot reach each other
```

| Network | Type | Role |
|---|---|---|
| `dock-net-proxy` | bridge (NAT enabled) | Proxy egress (internet reachability). Workers never attach |
| `dock-net-w-<name>` | bridge `Internal` (no NAT) | Per-worker; only the proxy is additionally connected |

- **Worker↔worker blocked**: each worker is on its own `Internal` network (separate L2 segment), so they cannot reach one another.
- **Worker→host/internet blocked**: `Internal: true` means no host route and no NAT. The only reachable peer is the proxy.
- **Worker→proxy**: via Docker embedded DNS (`codex-auth-proxy`) on the shared network, reaching only the data-plane port (18080). `/admin/*` (token issuance, etc.) lives on a separate listener that is **bound to the proxy's egress-network IP**, so it is unreachable from worker networks (different subnet → connection refused). The host reaches it only via the published port `127.0.0.1:18081`.
- **All egress via the proxy**: general traffic (git/npm/pip/curl) flows through the proxy's HTTP CONNECT forward proxy via `HTTP(S)_PROXY`; OpenAI/Anthropic API calls use the credential-injecting reverse routes.
- **Direct (non-proxy) outbound traffic times out — this is by design.** The worker network is `Internal` (no host route, no NAT), so anything that ignores `HTTP(S)_PROXY`, or any worker started with `--no-internet`, cannot reach the internet. `codex-dock run` injects `HTTP(S)_PROXY` automatically.

---

## Network Management Commands

### `codex-dock network create`
Creates the egress network (`dock-net-proxy`). `proxy run` also auto-creates it if missing.

### `codex-dock network status`
Shows the egress network state plus the list of per-worker networks currently present.

### `codex-dock network rm`
Removes the egress network. Per-worker networks are disconnected and removed automatically when a worker is removed.

### Per-worker network lifecycle
- **Foreground `codex-dock run`** (no `--detach`): the container and its dedicated network are removed automatically on exit, so networks don't accumulate. Pass `--keep` to retain them.
- **`--detach` (background)**: the container persists; its network is disconnected and removed when you `codex-dock rm <name>`.
- Auto-generated worker names are chosen to avoid colliding with existing containers/networks, preventing two workers from ever sharing one Internal network.

---

## Egress Control (Forward-Proxy Allowlist)

Use `codex-dock proxy run --forward-allow-domain <domain>` (repeatable) to restrict the forward proxy to specific domains (and their subdomains); everything else returns 403. Omitting it allows all destinations.

```bash
codex-dock proxy run \
  --forward-allow-domain github.com \
  --forward-allow-domain registry.npmjs.org \
  --forward-allow-domain pypi.org
```

`codex-dock run --no-internet` omits the `HTTP(S)_PROXY` vars for that worker (only the API reverse routes remain reachable; general egress is disabled).

---

## Notes

- Because no iptables are involved, **macOS / Windows (Docker Desktop) get the same isolation as Linux** (Docker Desktop manages the `Internal` network blocking rules).
- The old `codex-dock firewall` command and the `--allow-host`/`--block-host`/`--no-firewall`/`--sudo` flags have been removed.
