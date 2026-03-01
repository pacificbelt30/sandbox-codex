# codex-dock

**AI Sandbox Container Manager** — Runs [Codex CLI](https://github.com/openai/codex) inside isolated Docker containers with auth proxy, network separation, and parallel worker support.

## Features

- **Security isolation**: Codex runs in a Docker container, not on your host
- **Auth Proxy**: API keys never touch the container; short-lived tokens are injected instead
- **dock-net**: Dedicated Docker bridge network with ICC disabled and host access blocked
- **git worktree**: Parallel development branches, each in their own container
- **dock-ui**: Terminal UI for managing all workers at a glance
- **Package management**: `apt`, `pip`, `npm` packages via `--pkg` or `packages.dock`

## Quick Start

```bash
# Build the base sandbox image
codex-dock build

# Set your API key
export OPENAI_API_KEY=sk-...
codex-dock auth set

# Run Codex in a sandbox (mounts current directory)
codex-dock run

# Run with a task, fully automated, in the background
codex-dock run --task "Write unit tests for auth module" --full-auto --detach

# Use git worktree on a feature branch
codex-dock run --worktree --branch feature-auth --new-branch

# Parallel workers (3 branches auto-created)
codex-dock run --parallel 3 --worktree

# Monitor all workers
codex-dock ui
```

## Commands

| Command | Description |
|---------|-------------|
| `codex-dock run` | Start a sandboxed Codex worker |
| `codex-dock ps` | List workers |
| `codex-dock stop` | Stop containers |
| `codex-dock rm` | Remove stopped containers |
| `codex-dock logs` | View container logs |
| `codex-dock ui` | Launch TUI dashboard |
| `codex-dock auth` | Manage authentication |
| `codex-dock network` | Manage dock-net |
| `codex-dock build` | Build sandbox image |

## Configuration

Default config: `~/.config/codex-dock/config.toml`

See `configs/config.toml.example` for all options.

## Security

- Containers run as non-root (uid:1000)
- `--cap-drop ALL` applied
- `auth.json` is never mounted into containers
- Tokens expire on container stop or TTL expiry
- Container↔container and container→host traffic blocked via ICC and iptables

## Requirements

- Go 1.22+
- Docker Engine

## Build

```bash
go build -o codex-dock .
```
