# Quick start

[日本語](../getting-started.md) | **English**

This guide assumes an administrator has completed the [Kata / Dragonball host setup](../setup.md) (Japanese).
The host needs Linux x86_64, KVM, cgroup v2, dedicated containerd, Kata runtime-rs, Dragonball, BuildKit and `buildctl`.

## 1. Check the host

Run as a member of the operator group. Use the same account for credentials and normal CLI operations.

```bash
cxd runtime-check
```

To build from a source checkout, use Go 1.24.7 or later and Make:

```bash
make build
make install-config
make install-templates
```

This builds the CLI and helper binaries. Host service installation is a separate administrator task.

## 2. Prepare credentials

Configure credentials before starting the proxy.

| Agent | Host credential sources |
|---|---|
| Codex | `OPENAI_API_KEY`, saved API key, `~/.codex/auth.json` |
| Claude Code | `ANTHROPIC_API_KEY`, saved API key, `~/.claude/.credentials.json` |

Use an existing login file under the caller's home, or set an API key in the environment.
See [credential precedence](auth-proxy.md#credential-sources).
The auth proxy runs as the caller's UID/GID and reads files through read-only binds.
Owner-only `0600` files work without changing their permissions.
Real API keys and OAuth access/refresh tokens are not passed to workers.

## 3. Prepare images and start the proxies

From the repository root, run the guided preparation command as your normal account:

```bash
cxd init
# For Claude Code:
cxd init --agent claude
```

It checks host readiness and credentials, builds a missing worker image, checks image compatibility, and starts the proxies. If the proxy image is missing, its build needs the Go source in the repository root. After building it once, you can return to your project directory.

To perform those steps separately:

```bash
cxd proxy build
cxd proxy run
cxd build
```

`proxy run` creates the network and starts two proxies:
`codex-auth-proxy` for authenticated AI requests, and `codex-http-proxy` for general HTTP/CONNECT traffic.
Only the admin port is published to host loopback (`127.0.0.1:18081`).

## 4. Start a worker

Run from the project directory, which is shared at `/workspace`.
To share a repeatable worker image and defaults with a team, run `cxd project init` at the repository root and commit `codex-dock.toml`. Point `[build]` at a project Dockerfile for extra tools. After cloning, prepare the host and credentials, then run `cxd project check` and `cxd init`. See [project settings](configuration.md#project-settings).

```bash
cxd codex
# Or:
cxd run --agent claude

# Omit --agent for a shell with proxy authentication configured.
cxd run
cxd run -- ls -la
```

The default runtime is Kata. A failed isolation check stops startup; it never falls back to runc automatically.

## 5. Run in the background

```bash
cxd run --agent codex --name add-tests \
  --task "Add unit tests" --approval-mode full-auto \
  --worktree --branch add-tests --new-branch -d

cxd ps -a
cxd logs add-tests --follow
```

`full-auto` disables agent approval prompts. Shared files remain writable according to their mount permissions.

## 6. Stop, restart and remove

```bash
cxd stop add-tests
cxd start add-tests
cxd rm add-tests --force
```

Stop revokes the worker token and keeps the container and snapshot for restart.
Restart rechecks isolation and restores authentication for the same worker ID and workspace.
Save required results before removal, which also cleans up managed worktrees and networks.

Foreground workers are removed when they finish unless `--keep` is set.
Shared proxies keep running. See [operations](../operations.md) and [troubleshooting](../troubleshooting.md) (Japanese).
