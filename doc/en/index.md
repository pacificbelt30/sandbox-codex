# codex-dock Documentation

> [日本語](../index.md) | **English**

**codex-dock** is an **AI Sandbox Container Manager** that runs [Codex CLI](https://github.com/openai/codex) safely inside Docker containers.
It provides an Auth Proxy that isolates credentials from containers, a dedicated bridge network, and parallel worker management.

---

## Documentation Index

| Document | Description |
|---|---|
| [Architecture Overview](architecture.md) | System architecture diagram and component descriptions |
| [Auth Proxy Specification](auth-proxy.md) | Auth proxy technical details and flow diagrams |
| [Network Specification](network.md) | dock-net configuration and security policy |
| [Command Reference](commands.md) | All commands and options |
| [Configuration Reference](configuration.md) | All config.toml settings |
| [Quick Start](getting-started.md) | From installation to first run |

---

## System Overview

```
┌─────────────────────────────────────────────────────────────┐
│  Host Environment                                            │
│                                                              │
│  ┌──────────────┐    ┌────────────────────────────────────┐ │
│  │  codex-dock  │    │  Auth Proxy (127.0.0.1:PORT)       │ │
│  │  (CLI)       │───▶│  - Issues short-lived tokens       │ │
│  └──────────────┘    │  - Protects API keys / OAuth creds │ │
│         │            └──────────┬─────────────────────────┘ │
│         │                       │                            │
│         ▼                       │                            │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  dock-net (192.168.200.0/24)  Docker bridge network  │   │
│  │                                                       │   │
│  │  ┌──────────────┐  ┌──────────────┐                  │   │
│  │  │ Container A  │  │ Container B  │  (ICC disabled)  │   │
│  │  │ codex-dock   │  │ codex-dock   │◀─ inter-container│   │
│  │  │ worker-1     │  │ worker-2     │   comm blocked   │   │
│  │  └──────────────┘  └──────────────┘                  │   │
│  └──────────────────────────────────────────────────────┘   │
│                              │ IP Masquerade                 │
│                              ▼                               │
│                        Internet                              │
│                        (OpenAI API, etc.)                    │
└─────────────────────────────────────────────────────────────┘
```

---

## Key Features

- **Security Isolation**: Codex runs inside Docker containers, not on the host
- **Auth Proxy**: API keys never reach containers directly — protected via short-lived tokens
- **dock-net**: Dedicated bridge network with ICC disabled and host access restricted
- **git worktree**: Parallel development branches each running in their own container
- **dock-ui**: Terminal UI for monitoring all workers at a glance
- **Package management**: Install `apt`, `pip`, `npm` packages via `--pkg` or `packages.dock`

## Requirements

- Go 1.24 or later
- Docker Engine 20.10 or later
