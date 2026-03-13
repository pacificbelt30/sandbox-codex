# codex-dock Documentation

> [日本語](../index.md) | **English**

**codex-dock** is an **AI Sandbox Container Manager** that runs [Codex CLI](https://github.com/openai/codex) safely inside Docker containers.
It provides an Auth Proxy that isolates credentials from containers, a dedicated bridge network, and parallel worker management.

---

## Documentation Index

### Getting Started

| Document | Description |
|---|---|
| [Quick Start](getting-started.md) | From installation to first run |
| [Architecture Overview](architecture.md) | System diagram, components, startup sequence |
| [Security Design](security.md) | Container settings, protections, known issues |

### Auth Proxy

| Document | Description |
|---|---|
| [Auth Proxy Overview & Deployment](auth-proxy.md) | How the proxy works, startup, auth modes |
| [API Endpoint Reference](auth-proxy/endpoints.md) | Full request/response spec for all endpoints |
| [Token Lifecycle & Security](auth-proxy/tokens.md) | Token lifecycle and security considerations |
| [Using Auth Proxy Standalone](proxy-standalone.md) | Configure Codex CLI without `codex-dock run` ✨ |

### Network

| Document | Description |
|---|---|
| [Network Specification](network.md) | dock-net configuration, security policy, troubleshooting |

### Command Reference

| Document | Description |
|---|---|
| [Command Reference (Index)](commands.md) | All commands index and global options |
| [`codex-dock run`](commands/run.md) | Container startup, approval modes, parallel execution |
| [`codex-dock proxy`](commands/proxy.md) | Auth Proxy build / run / serve / stop / rm |
| [Worker Management (ps / stop / rm / logs)](commands/worker.md) | List, stop, remove, view logs |
| [`codex-dock auth`](commands/auth.md) | Auth show / set / rotate |
| [`codex-dock network`](commands/network-cmd.md) | dock-net create / rm / status |
| [`codex-dock build`](commands/build.md) | Build sandbox image |
| [`codex-dock ui`](commands/ui.md) | TUI dashboard key bindings |

### Configuration

| Document | Description |
|---|---|
| [Configuration Reference](configuration.md) | All config.toml settings, env vars, auth files |

---

## System Overview

```
┌─────────────────────────────────────────────────────────────┐
│  Host Environment                                            │
│                                                              │
│  ┌──────────────┐    ┌────────────────────────────────────┐ │
│  │  codex-dock  │    │  Auth Proxy (0.0.0.0:PORT)         │ │
│  │  (CLI)       │───▶│  - Issues short-lived tokens       │ │
│  └──────────────┘    │  - Protects API keys / OAuth creds │ │
│         │            └──────────┬─────────────────────────┘ │
│         │                       │ host.docker.internal:PORT  │
│         ▼                       │                            │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  dock-net (10.200.0.0/24)  Docker bridge network     │   │
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
