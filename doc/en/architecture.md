# Architecture Overview

> [日本語](../architecture.md) | **English**

codex-dock is composed of **4 main components**.

---

## Component Structure

```
codex-dock
├── cmd/                     CLI commands (cobra)
│   ├── run.go               Container launch and worker management
│   ├── auth.go              Auth configuration (auth set / auth status)
│   ├── build.go             Sandbox image build
│   ├── ps.go / stop.go / rm.go / logs.go
│   ├── ui.go                TUI dashboard launch
│   └── network.go           dock-net management
│
├── internal/
│   ├── authproxy/           Auth Proxy
│   │   ├── proxy.go         HTTP server and token management
│   │   └── auth.go          API key / OAuth credential loading
│   ├── sandbox/             Docker container management
│   │   ├── manager.go       Container lifecycle
│   │   ├── types.go         RunOptions and type definitions
│   │   └── packages.go      Package definition parsing
│   ├── network/             dock-net management
│   │   └── manager.go       Bridge network create / delete
│   ├── worktree/            git worktree management
│   │   └── worktree.go      Worktree create / delete
│   └── ui/                  Terminal UI (Bubble Tea)
│       └── ui.go
│
└── docker/
    ├── Dockerfile           Sandbox image definition
    └── entrypoint.sh        Container startup script (includes auth retrieval)
```

---

## Startup Sequence

The following shows the processing flow when `codex-dock run` is executed.

```
User               codex-dock CLI          Auth Proxy              Docker / Container
  │                    │                       │                         │
  │  codex-dock run    │                       │                         │
  │──────────────────▶│                       │                         │
  │                    │                       │                         │
  │                    │ 1. Ensure dock-net     │                         │
  │                    │──────────────────────────────────────────────▶ │
  │                    │                       │                         │
  │                    │ 2. Start Auth Proxy    │                         │
  │                    │──────────────────────▶│                         │
  │                    │  (0.0.0.0:PORT)       │                         │
  │                    │                       │                         │
  │                    │ 3. Issue short-lived token                       │
  │                    │──────────────────────▶│                         │
  │                    │◀── cdx-xxxx...        │                         │
  │                    │                       │                         │
  │                    │ 4. Create & start container                      │
  │                    │  CODEX_TOKEN=cdx-xxx  │                         │
  │                    │  OPENAI_BASE_URL=      │                         │
  │                    │  http://host.docker.  │                         │
  │                    │    internal:PORT/v1   │                         │
  │                    │  CODEX_REFRESH_TOKEN_ │                         │
  │                    │   URL_OVERRIDE=...    │                         │
  │                    │──────────────────────────────────────────────▶ │
  │                    │                       │                         │
  │                    │                       │  5. GET /token          │
  │                    │                       │◀────────────────────────│
  │                    │                       │  X-Codex-Token: cdx-xxx │
  │                    │                       │─────────────────────── ▶│
  │                    │                       │  {api_key or            │
  │                    │                       │   oauth_access_token}   │
  │                    │                       │                         │
  │                    │                       │  6. Launch Codex CLI    │
  │                    │                       │     (auth.json written)  │
  │                    │                       │                         │
  │                    │                       │  7. POST /v1/responses  │
  │                    │                       │◀────────────────────────│
  │                    │                       │  ↓ forwarded to         │
  │                    │                       │  api.openai.com/v1 or   │
  │                    │                       │  chatgpt.com/backend-api│
  │◀──────────────────────────────────────────────────────────────────── │
  │  Container output  │                       │                         │
```

---

## Security Design Principles

codex-dock's security is based on the principle of **"never pass secrets directly to containers"**.

```
                    BAD (traditional approach)
┌────────────────┐                           ┌──────────────────────┐
│      Host      │  OPENAI_API_KEY=sk-xxx   │     Container        │
│                │─────────────────────────▶│  (risk: key exposed) │
└────────────────┘                           └──────────────────────┘

                    GOOD (codex-dock approach)
┌─────────────────────────────────────────────────────────────────────┐
│   Host                                                               │
│                                                                      │
│  API Key ──▶ Auth Proxy ──▶ Placeholder ──▶ Container               │
│  (protected)  (0.0.0.0)    (cdx-xxxx)       TTL-scoped              │
│               ↑ reachable via host.docker.internal:PORT             │
│                   │                                                  │
│                   │ On every API request:                            │
│                   │ replaces Authorization with real credentials     │
│                   ▼                                                  │
│              api.openai.com / chatgpt.com                            │
│                                                                      │
│  ・Real API Key / access_token never reaches containers              │
│  ・Containers only hold a placeholder (cdx-xxxx)                    │
│  ・Placeholder cannot be used to access OpenAI directly             │
│  ・OAuth refresh_token is kept on host only                         │
└─────────────────────────────────────────────────────────────────────┘
```

---

## Container Security Settings

Each sandbox container is launched with the following security settings:

| Setting | Value | Effect |
|---|---|---|
| `--cap-drop ALL` | Drop all Linux capabilities | Prevents privilege escalation |
| `--security-opt no-new-privileges` | No new privileges allowed | Prevents setuid/setgid abuse |
| `USER codex (uid:1000)` | Runs as non-root user | Prevents root-level host operations |
| `--pids-limit 512` | Max 512 processes | Prevents fork bombs |
| Network: `dock-net` | Bridge network with ICC disabled | Blocks inter-container communication |

---

## Authentication Modes

codex-dock supports two authentication methods: **API key mode** and **OAuth mode**.

```
[API Key Mode]
Host (Auth Proxy)                    Container
  Holds API key in memory
  ↓
  CODEX_TOKEN=cdx-xxx        ──────▶ GET /token → {"api_key": "cdx-xxx"}  ← placeholder
  OPENAI_BASE_URL=proxy/v1           export OPENAI_API_KEY=cdx-xxx  ← dummy
                                     exec codex
                                     ↓
  POST /v1/responses ◀───────────────  Codex CLI (Authorization: Bearer cdx-xxx)
  Replace Authorization: sk-xxx      ← proxy injects real key
  Forward to: api.openai.com/v1

[OAuth Mode (ChatGPT subscription)]
Host (Auth Proxy)                    Container
  ~/.codex/auth.json
  access_token, id_token             CODEX_TOKEN=cdx-xxx
  refresh_token (host only) ────────▶ GET /token
                                       → {oauth_access_token: "cdx-xxx",  ← placeholder
                                           id_token: "ey...",              ← real
                                           ...}
                                       write ~/.codex/auth.json
                                         access_token: "cdx-xxx"  ← dummy
                                         refresh_token: ""
                                       write ~/.config/codex/config.toml
                                         chatgpt_base_url=proxy/chatgpt/
                                       exec codex
                                     ↓
  POST /v1/responses ◀───────────────  Codex CLI (Authorization: Bearer cdx-xxx)
  Replace Authorization with real access_token  ← proxy injects
  Also overwrites ChatGPT-Account-Id with correct value
  Forward to: chatgpt.com/backend-api/codex
  ↓
  POST /oauth/token?cdx=xxx ◀────────  Codex CLI (every 8 hours)
  Injects host refresh_token and
  forwards to auth.openai.com/oauth/token
  Replaces access_token → "cdx-xxx" (placeholder) before returning
  (refresh_token excluded)
```

> **Important**: In OAuth mode, neither `refresh_token` nor the real `access_token` reaches the container.
> The container holds only the placeholder (`cdx-xxx`), which cannot be used to access OpenAI directly.
> Once `CODEX_TOKEN` expires, token refresh via the proxy becomes impossible as well.

---

## Implementation Status Summary

| Category | Implemented | Partial | Not Implemented |
|---|---|---|---|
| Auth (AUTH) | F-AUTH-01–05, 07 | F-AUTH-06 | F-AUTH-08 |
| Network (NET) | F-NET-01, 03, 04, 05, 06 | F-NET-02 | — |
| Packages (PKG) | F-PKG-01–04, 06 | F-PKG-05 | — |
| Worktree (WT) | F-WT-01–04 | — | F-WT-05 |
| UI | F-UI-01 | F-UI-02, 03 | F-UI-04, 05 |
| Security (SEC) | NF-SEC-02, 03, 04 | NF-SEC-05 | NF-SEC-01, 06 |

> F-AUTH-04 (auto-revoke token on container stop): resolved — `sandbox.Manager.Stop()` calls `proxy.RevokeToken()`.
> F-AUTH-07 (OAuth refresh relay): implemented via `/oauth/token` proxy endpoint.
> F-NET-04 (Auth Proxy unreachable from containers): resolved — Auth Proxy binds to `0.0.0.0` and worker containers get `--add-host=host.docker.internal:host-gateway`. Containers reach the proxy via `http://host.docker.internal:PORT`.

See [Auth Proxy Specification](auth-proxy.md) and [Network Specification](network.md) for details.
