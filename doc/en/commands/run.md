# `codex-dock run` — Start a Sandbox Container

> [日本語](../../commands/run.md) | **English**
>
> [← Command Reference](../commands.md)

Runs Codex CLI inside a Docker container with Auth Proxy and network isolation configured automatically.

> **Automatic image build**: If the image specified by `--image` does not exist locally, it is built automatically using the same logic as `codex-dock build`.

```bash
codex-dock run [OPTIONS]
```

---

## Options

| Option | Short | Default | Description |
|---|---|---|---|
| `--image` | `-i` | `codex-dock:latest` | Docker image for the sandbox |
| `--pkg` | `-p` | | Additional packages (repeatable): `apt:<pkg>`, `pip:<pkg>`, `npm:<pkg>` |
| `--pkg-file` | | | Path to a package definition file (`packages.dock`) |
| `--project` | `-d` | `.` (current dir) | Directory to mount at `/workspace` |
| `--worktree` | `-w` | `false` | Use git worktree to isolate containers |
| `--branch` | `-b` | | Branch name to check out (requires `--worktree`) |
| `--new-branch` | `-B` | `false` | Create a new branch (requires `--worktree` and `--branch`) |
| `--name` | `-n` | auto-generated | Container name (defaults to `codex-<adj>-<noun>`) |
| `--task` | `-t` | | Initial task prompt passed to Codex |
| `--approval-mode` | | `suggest` | Codex CLI approval mode ([details](#--approval-mode)) |
| `--full-auto` | | `false` | **Deprecated**: use `--approval-mode full-auto` |
| `--model` | `-m` | | Model name to pass to Codex |
| `--read-only` | | `false` | Mount project directory read-only |
| `--no-internet` | | `false` | Disable internet access inside container |
| `--token-ttl` | | `3600` | Auth Proxy token TTL in seconds |
| `--agents-md` | | | Path to `AGENTS.md` file |
| `--detach` | `-D` | `false` | Run in background (no log output) |
| `--parallel` | `-P` | `1` | Number of parallel workers |
| `--user` | | `""` | Container execution user ([details](#--user)) |

---

## `--approval-mode`

Controls how Codex CLI requests approval for actions. Designed with Docker container isolation in mind.

| Value | Codex CLI Flag | Behavior |
|---|---|---|
| `suggest` (default) | none | Prompt for every action (safest) |
| `auto-edit` | `--ask-for-approval unless-allow-listed` | Auto-apply file edits; prompt for command execution |
| `full-auto` | `--ask-for-approval never` | Never prompt for approval |
| `danger` | `--dangerously-bypass-approvals-and-sandbox` | Bypass all approvals and sandbox restrictions |

> **About `danger` mode**: Disables Codex CLI's built-in sandbox, but the Docker container itself provides the isolation boundary (`--cap-drop ALL`, dedicated network, pids-limit, etc.). Host environment is not affected.

---

## `--user`

Runs the container process as a specific uid:gid for filesystem permission consistency.

| Value | Behavior |
|---|---|
| `""` (omitted) | Image default user (`codex` uid:1001) |
| `current` | Automatically uses the uid:gid of the user running `codex-dock` |
| `dir` | Automatically uses the uid:gid of the `--project` directory owner |
| `uid` or `uid:gid` | Explicit value (e.g., `1000`, `1000:1000`) |

---

## Package Format

```
apt:libssl-dev          # install via apt
pip:requests            # install via pip
npm:lodash              # install via npm
libssl-dev              # no prefix → treated as apt (default)
```

---

## Parallel Workers

```bash
codex-dock run --parallel 3 --worktree --branch myfeature
```

Automatically creates branches: `myfeature-1`, `myfeature-2`, `myfeature-3`.

---

## Examples

```bash
# Basic run (mount current directory)
codex-dock run

# Run with task in full-auto mode
codex-dock run --task "Write unit tests" --approval-mode full-auto

# Use git worktree on a feature branch
codex-dock run --worktree --branch feature-auth --new-branch

# Run 3 parallel workers
codex-dock run --parallel 3 --worktree

# Install additional packages
codex-dock run --pkg "apt:libssl-dev" --pkg "pip:requests"

# Run in background
codex-dock run --task "Refactor" --approval-mode full-auto --detach

# Secure run: read-only, no internet
codex-dock run --read-only --no-internet --task "Code review"
```

---

## Related Documentation

- [Quick Start](../getting-started.md)
- [Architecture Overview](../architecture.md) — Startup sequence
- [Auth Proxy Specification](../auth-proxy.md)
- [Network Specification](../network.md)
- [Configuration Reference](../configuration.md)
