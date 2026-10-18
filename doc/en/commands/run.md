# `codex-dock run` — Start a Sandbox Container

> [日本語](../../commands/run.md) | **English**
>
> [← Command Reference](../commands.md)

Runs Codex CLI, Claude Code or a shell in a Kata VM, managed by containerd with Dragonball.
The default is `--runtime kata`. Dedicated Linux bridges, namespaces and nftables block direct access; selected proxies provide outbound routes.
The execution host is Linux x86_64. See [quick start](../getting-started.md) and [network](../network.md).

Missing images are built automatically from the matching template or default Dockerfile.
On-disk template rebuilds are controlled by [auto_build](../configuration.md#auto_build).
See [build](build.md) for custom tags and templates.

## Launch Codex CLI directly

```bash
codex-dock codex
codex-dock codex "Inspect this repository"
codex-dock codex exec --json "Summarize the changes"
```

Arguments are forwarded verbatim to Codex. The current directory is mounted at `/workspace`, and the worker execs Codex without an interactive shell wrapper. Configure worker defaults under `[run]`; use `codex-dock run --agent codex` for image, worktree, mount and detach options. Starting a fresh VM takes longer than running Codex on the host.

```bash
codex-dock run [OPTIONS] [IMAGE [COMMAND...]]
codex-dock run [OPTIONS] -- COMMAND...  # Run in the default image
```

IMAGE and COMMAND follow Docker's positional order. Put codex-dock flags before IMAGE.
With `--image` or `--template`, positional arguments are treated as the command.
The command is equivalent to `--exec` without quoting it as one string.

```bash
codex-dock run -d --name worker codex-dock:latest
codex-dock run codex-dock:latest npm test -- --watch=false
codex-dock run -- npm test -- --watch=false
```

---

## Options

| Option | Short | Default | Description |
|---|---|---|---|
| `--runtime` | | `kata` | Worker isolation; `runc` is explicit compatibility mode |
| `--kata-runtime` | | `io.containerd.kata.v2` | Kata registration on dedicated containerd |
| `--image` | `-i` | `codex-dock:latest` | OCI image for the sandbox; also accepted positionally |
| `--template` | `-T` | | Sandbox image template (`plain`, `pwn`, etc.). Resolved to the template's image tag; an explicit `--image` always takes precedence. Auto-built if not present locally |
| `--pkg` | `-p` | | Additional packages (repeatable): `apt:<pkg>`, `pip:<pkg>`, `npm:<pkg>` |
| `--pkg-file` | | | Path to a package definition file (`packages.dock`) |
| `--project` | | `.` (current dir) | Directory to mount at `/workspace` |
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
| `--no-internet` | | `false` | Disconnect general egress; keep fixed AI API routes |
| `--keep` | | `false` | Keep the container and its per-worker network after a foreground run exits (default: remove them so networks don't accumulate) |
| `--force-recreate-proxy` | | `false` | Recreate the Auth Proxy when its credential state differs from the host even if workers are running (existing workers lose authentication) |
| `--token-ttl` | | `3600` | Auth Proxy token TTL in seconds |
| `--agents-md` | | | Path to `AGENTS.md` file |
| `--detach` | `-d` | `false` | Run in background (no log output); the old `-D` still works |
| `--parallel` | `-P` | `1` | Number of parallel workers |
| `--shell` | `-s` | `false` | Start a raw (unauthenticated) shell, bypassing `entrypoint.sh` (debugging) |
| `--shell-type` | | auto-detected from `$SHELL`, else `bash` | Interactive shell to launch ([details](#--shell-type)) |
| `--user` | | `current` | Container execution user ([details](#--user)) |

When the host and Auth Proxy credential modes differ, `run` automatically
recreates the Auth Proxy if no workers are running. If workers are running it
stops with an error so their in-memory tokens are not invalidated. Pass
`--force-recreate-proxy` to recreate it anyway, then create new workers if their pinned proxy IP changed.

---

## Docker CLI differences

`run -d`, `run IMAGE COMMAND`, `ps -a`, `logs -f`, `start`, `stop`, `rm -f`, and `build -t` use Docker style.
Replace the old `run -d PATH` project shorthand with `run --project PATH`. The old `-D` still means detach.
In this CLI, `run -p` selects packages, `run -i` selects an image, `run -t` sets an agent task, and `-v` enables verbose output.
Host port publishing is not supported. Use `--mount HOST:CONTAINER[:ro|rw]` for extra bind mounts.
The current project is shared at `/workspace` by default, and a foreground worker is removed after exit unless you pass `--keep`.

---

## `--approval-mode`

Controls how Codex CLI requests approval for actions. The selected worker runtime provides the outer isolation boundary.

| Value | Codex CLI Flag | Behavior |
|---|---|---|
| `suggest` (default) | No approval flag | Use the agent's defaults |
| `auto-edit` | `--ask-for-approval on-request` | The model decides when to ask for approval |
| `full-auto` | `--ask-for-approval never` | Never prompt for approval |
| `danger` | `--dangerously-bypass-approvals-and-sandbox` | Bypass all approvals and sandbox restrictions |

In a verified Kata worker, Codex's inner sandbox is set to `danger-full-access` to avoid nested namespace/seccomp conflicts.
Approval settings are separate; noninteractive exec may choose its own approval behavior.
Shared files remain writable according to mount permissions. See [security](../security.md).

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
binary isn't present in the image — the engine fails to start the
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
codex-dock run --parallel 2 --worktree --branch myfeature
```

The default node limit is 2 simultaneous workers. Creates branches: `myfeature-1`, `myfeature-2`.

---

## Examples

```bash
# Basic run (mount current directory)
codex-dock run

# Run with task in full-auto mode
codex-dock run --agent codex --task "Write unit tests" --approval-mode full-auto

# Use git worktree on a feature branch
codex-dock run --worktree --branch feature-auth --new-branch

# Run 2 parallel workers
codex-dock run --parallel 2 --worktree

# Install additional packages
codex-dock run --pkg "apt:libssl-dev" --pkg "pip:requests"

# Run in background
codex-dock run --agent codex --task "Refactor" --approval-mode full-auto --detach

# Secure run: read-only, no internet
codex-dock run --agent codex --read-only --no-internet --task "Code review"

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
