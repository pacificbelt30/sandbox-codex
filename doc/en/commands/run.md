# `codex-dock run` — Start a Sandbox Container

> [日本語](../../commands/run.md) | **English**
>
> [← Command Reference](../commands.md)

Runs Codex CLI inside a Docker container with Auth Proxy and network isolation configured automatically.

> **Network isolation**: on start, a per-worker Docker `Internal` network (`dock-net-w-<name>`) is created and the Auth Proxy is connected to it. Worker↔worker, worker→host, and worker→internet traffic is blocked by Docker (no `iptables`/`sudo`; same on macOS / Windows).

> **Automatic image build**: If the image specified by `--image` (or `--template`) does not exist locally, it is built automatically. When the tag matches a known template (e.g. `codex-dock:pwn`), the template is used; otherwise it falls back to the same logic as `codex-dock build`. This also applies to user templates under `~/.config/codex-dock/templates/` — see the [User Templates section of `codex-dock build`](build.md#user-templates).
>
> On top of that, when a template's Dockerfile exists as a real file on disk (a user template, or a built-in one copied there via `make install-templates`), its modification time is compared against the built image's creation timestamp — **if the file is newer, the image is rebuilt automatically** before the container starts. This is controlled by the `auto_build` (global) / `run.auto_build` (`run`-only) config keys, enabled by default; see the [Configuration Reference](../configuration.md#auto_build). Built-in templates that remain purely embedded in the binary have no file to compare against, so this doesn't apply to them.

```bash
codex-dock run [OPTIONS]
```

---

## Options

| Option | Short | Default | Description |
|---|---|---|---|
| `--image` | `-i` | `codex-dock:latest` | Docker image for the sandbox |
| `--template` | `-T` | | Sandbox image template (`plain`, `pwn`, etc.). Resolved to the template's image tag; an explicit `--image` always takes precedence. Auto-built if not present locally |
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
| `--mount` | | | Additional host directory to mount (repeatable): `<host-path>:<container-path>[:ro\|rw]` ([details](#--mount)) |
| `--no-internet` | | `false` | Disable general egress (omit `HTTP(S)_PROXY`; only the proxy's API reverse routes remain reachable) |
| `--keep` | | `false` | Keep the container and its per-worker network after a foreground run exits (default: remove them so networks don't accumulate) |
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

## `--mount`

`--mount` bind-mounts an arbitrary host directory into the container, in addition to
`/workspace` (`--project`). Repeat the flag to mount multiple directories.

```
--mount <host-path>:<container-path>[:ro|rw]
```

- `<host-path>`: directory on the host (relative paths are resolved against the current
  working directory when `codex-dock run` is invoked)
- `<container-path>`: absolute path inside the container; `/workspace` is reserved for
  `--project` and cannot be used here
- `:ro` / `:rw`: read-only / read-write (defaults to `rw` when omitted)
- Mounting the same `<container-path>` more than once is an error

```bash
# Mount a shared dataset directory read-only
codex-dock run --mount /home/user/datasets:/data:ro

# Mount multiple directories, mixing read-write and read-only
codex-dock run --mount ./cache:/cache --mount /opt/models:/models:ro
```

> Additional mounts can be independently read-only regardless of `/workspace`.
> `--read-only` only affects the `/workspace` mount.

This can also be set as a default via `mounts` in the `[run]` section of the config
file (`~/.config/codex-dock/config.toml`); an explicit `--mount` always overrides it.
See [`doc/en/configuration.md`](../configuration.md#runmounts) for details.

```toml
[run]
mounts = ["/home/user/datasets:/data:ro", "./cache:/cache"]
```

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

# Use a template image (auto-built if not present)
codex-dock run --image codex-dock:pwn

# Same thing via --template
codex-dock run --template pwn

# Works for your own templates under ~/.config/codex-dock/templates/ too
codex-dock run --template mytool
```

---

## Related Documentation

- [Quick Start](../getting-started.md)
- [Architecture Overview](../architecture.md) — Startup sequence
- [Auth Proxy Specification](../auth-proxy.md)
- [Network Specification](../network.md)
- [Configuration Reference](../configuration.md)
