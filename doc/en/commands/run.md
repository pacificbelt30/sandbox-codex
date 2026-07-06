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
codex-dock run [OPTIONS] [-- command...]
```

A command placed after `--` runs once instead of the interactive shell — like
`docker run <image> <cmd>` — and is equivalent to `--exec` but without having
to quote the whole command as one string ([details](#--exec)).

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
| `--agent` | | `""` (shell) | AI agent to launch inside the sandbox: `""` (auth-configured shell), `codex`, or `claude` |
| `--task` | `-t` | | Initial task prompt passed to Codex |
| `--exec` | `-e` | | Command to run once instead of an interactive shell when `--agent` is omitted ([details](#--exec)) |
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
| `--shell` | `-s` | `false` | Start a raw (unauthenticated) shell, bypassing `entrypoint.sh` (debugging) |
| `--shell-type` | | auto-detected from `$SHELL`, else `bash` | Interactive shell to launch ([details](#--shell-type)) |
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

## `--exec`

Runs a single command inside the sandbox instead of an interactive shell.
Passed into the container as the `DOCK_EXEC_CMD` environment variable, which
`entrypoint.sh` only consults in its no-agent branch (the auth-configured
shell case).

- If `--agent` is given, the agent **always takes priority** and `--exec` is ignored.
- Combining it with `--shell` (the raw debug shell that bypasses `entrypoint.sh`
  entirely) is an error, since `entrypoint.sh` — and therefore `--exec` — never runs.
- The command runs via the shell selected by `--shell-type` (default `bash`), through its `-c` flag.
- Mutually exclusive with a trailing `-- command...` on the command line — use whichever is
  more convenient; the latter avoids having to quote a multi-word command as a single string.

```bash
# Run one command in the auth-configured environment, then exit
codex-dock run --exec "ls -la"

# Equivalent, without needing to quote the whole command as one string
codex-dock run -- ls -la

# Run in the background
codex-dock run --exec "npm test" --detach

# --agent takes priority over --exec, so this launches Codex CLI, not `ls -la`
codex-dock run --agent codex --exec "ls -la"
```

---

## `--shell-type`

Selects the shell binary used for the sandbox's interactive shell: both the
auth-configured shell (when `--agent` is omitted) and the `--shell` debug
mode. Passed into the container as the `DOCK_SHELL` environment variable,
which `entrypoint.sh` reads.

| Value | Behavior |
|---|---|
| `bash` | Launches bash |
| `zsh` | Launches zsh; requires zsh to be installed in the image |

When neither the flag nor a config value (`default_shell_type` /
`run.shell_type`) sets a value, it is auto-detected from the basename of the
host's `$SHELL` environment variable — `zsh` maps to `zsh`, anything else
(including an unset or unrecognized `$SHELL`) falls back to `bash`. **This
auto-detection does not apply to `--shell` debug mode**, which always
defaults to `bash` when the flag is omitted: `--shell` overrides the
container's `Entrypoint` directly with the shell binary, bypassing
`entrypoint.sh` entirely, so there's no way for it to fall back if that
binary isn't present in the image — Docker just fails to start the
container. Pass `--shell --shell-type zsh` explicitly if you want zsh there.

The built-in `plain` template includes zsh, and `pwn` inherits it (`FROM
codex-dock:latest`), so `--shell-type zsh` works out of the box with either.
For a custom template, install zsh in its Dockerfile to use it. Outside
`--shell` mode (i.e. the no-agent shell and `--exec`), `entrypoint.sh` checks
whether the requested shell binary actually exists before using it and logs a
warning and falls back to bash if not, so a missing zsh there never fails the
container outright.

> The job-control wrapper that lets Ctrl+Z suspend an agent (`--agent
> codex`/`claude`) back to a shell prompt (`launch_interactive`) always uses
> bash internally, regardless of `--shell-type` — it relies on bash's
> `PROMPT_COMMAND`. `--shell-type` only affects the no-agent interactive shell
> and `--shell` debug mode.

```bash
# Launch the interactive shell with zsh
codex-dock run --shell-type zsh

# Raw debug shell with zsh (bypasses entrypoint.sh)
codex-dock run --shell --shell-type zsh
```

The default can be changed via `default_shell_type` (global) / `run.shell_type`
(`run`-only) in `~/.config/codex-dock/config.toml` — see the
[Configuration Reference](../configuration.md#default_shell_type).

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
