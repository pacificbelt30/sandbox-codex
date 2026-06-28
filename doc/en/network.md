# Network Specification (Proxy Router + Per-Worker Networks)

> [日本語](../network.md) | **English**

codex-dock enforces network isolation using **Docker-native primitives only** — no `iptables`, no `sudo`. The isolation rules are managed by the Docker daemon (already root).

---

## Topology (two proxies)

Egress is split into **two containers by role**. The **credential-holding auth proxy never forwards general traffic**; general traffic is handled only by the credentials-free egress proxy (least privilege).

```
                       ┌──────────── internet (public only)
        dock-net-proxy ─┤  ← egress bridge for BOTH proxies
                        │
        ┌───────────────┴───────────────┐
 ┌──────▼───────┐                ┌───────▼───────┐
 │ codex-auth-  │                │ codex-http-   │
 │ proxy        │                │ proxy         │
 │ :18080 reverse(/v1,/anthropic)│ :18082 forward(CONNECT/HTTP)
 │ :18081 admin                  │  + LAN block + allowlist
 │ ★fixed 3 upstreams only       │  ★no credentials
 └──┬─────────┬─┘                └──┬─────────┬──┘
    │  (both multi-homed onto each worker net)│
 dock-net-w-A …                   dock-net-w-A …
        │                               │
     worker A:  OPENAI_BASE_URL→auth / HTTP_PROXY→http
```

| Network | Type | Role |
|---|---|---|
| `dock-net-proxy` | bridge (NAT enabled) | Egress for **both** proxies. Workers never attach |
| `dock-net-w-<name>` | bridge `Internal` (no NAT) | Per-worker; **both** proxies are additionally connected |

Proxy roles:

| Container | Role | Ports | Responsibility |
|---|---|---|---|
| `codex-auth-proxy` | `auth` | data 18080 / admin 18081 | Reverse routes (`/v1`, `/anthropic`, `/chatgpt`) that **inject the real credentials**; token issuance; admin. **Does NOT forward general traffic (CONNECT/absolute-URI → 405).** |
| `codex-http-proxy` | `egress` | 18082 | **Forward proxy only** (git/npm/pip). No credentials. Private/LAN block + domain allowlist. |

- **Worker↔worker blocked**: each worker is on its own `Internal` network (separate L2 segment).
- **Worker→host/internet blocked**: `Internal: true` means no host route and no NAT; the only reachable peers are the proxies.
- **Worker→proxy**: via Docker embedded DNS to `codex-auth-proxy:18080` (API) and `codex-http-proxy:18082` (general). The auth `/admin/*` lives on a separate listener **bound to the egress-network IP**, unreachable from worker networks (host-only via `127.0.0.1:18081`).
- **Egress split**: API (`OPENAI_/ANTHROPIC_BASE_URL`) → auth reverse routes (credential injection); general (`HTTP(S)_PROXY`) → http forward proxy. `NO_PROXY=codex-auth-proxy,…` keeps API/token traffic direct.
- **LAN block**: `codex-http-proxy` with `--block-private` refuses private/loopback/link-local destinations (RFC1918, 127/8, **169.254/16 = cloud metadata**, ULA, CGNAT) with 403. Enabled by default in `proxy run`; also applied to the auth proxy's upstream dials (defense in depth).
- **Direct (non-proxy) outbound traffic times out — by design.** `codex-dock run` injects `HTTP(S)_PROXY`; `--no-internet` omits it (only the auth API routes remain).

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
- **`codex-dock stop`**: only stops the container; the network is **kept on purpose** (needed to restart it). Removal happens on `rm`.
- **`codex-dock rm <name>` / TUI delete (D)**: removes the container, then force-disconnects every remaining endpoint on the dedicated network (including the proxy) and removes the network — reliably, even if the proxy isn't running.
- **`--detach` (background)**: the container persists; its network is removed when you `codex-dock rm`.
- Auto-generated worker names avoid colliding with existing containers/networks (the random-suffix fallback is re-checked too), so two workers never share one Internal network. With `--name`, Docker's container-name uniqueness rejects duplicates.

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

## Verification

A Docker-free smoke test exercises the core proxy/router behaviour (requires `go` / `python3` / `curl`):

```bash
bash scripts/smoke-proxy.sh
```

It checks: auth `/health`, `/admin/*` on the admin listener, `/admin/*` NOT on the data-plane port (split), **auth refusing to forward general traffic (405)**, the egress forward proxy (HTTP + CONNECT), and **`--block-private` blocking a LAN/loopback destination (403)**.

Container-level isolation (worker↔worker, Internal-network egress blocking) needs a Docker daemon. A Docker-gated E2E is included (it skips cleanly when Docker is unavailable):

```bash
bash scripts/e2e-docker.sh
```

It verifies that `proxy run` brings up both proxies with only the admin port host-published, and that a worker on its per-worker Internal network can reach the auth/http proxies but **not** the admin port, the internet directly, or another worker — and that the http proxy refuses private/LAN destinations (403).
