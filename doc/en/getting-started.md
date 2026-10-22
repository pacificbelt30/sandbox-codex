# Quick start

[日本語](../getting-started.md) | **English**

| Current state | Start here |
|---|---|
| No execution host yet | Complete [Installation](installation.md) first |
| Host is ready | Follow steps 1–4 below |
| Updating an existing installation | [Operations](../operations.md) (Japanese) |

## 1. Check the host

- Account: a normal user in the operator group.
- Use the same account for credentials and normal CLI operations.
- Normal `run` / `proxy run` operations do not need `sudo`.

```bash
oaiso runtime-check
```

| Task | Requirements |
|---|---|
| Run workers | Linux x86_64, KVM, cgroup v2, dedicated containerd, Kata runtime-rs, Dragonball |
| Build images | BuildKit and `buildctl` |
| Build the CLI from source | Go 1.24.7 or later and Make |

**From a source checkout**

```bash
make build
make install-config
make install-templates
```

- Builds the CLI and helper binaries.
- Existing configuration and templates are retained.
- Host services and pinned runtime assets require separate administrator installation.

## 2. Prepare credentials

**Choose one mode before starting the proxy.**

| Mode | Setup | Credential source |
|---|---|---|
| Independent profile | [Create a profile](commands/auth.md#independent-profiles); set global `auth_profile` and `[proxy].admin_secret_file` | Selected product Store only; no host fallback |
| Legacy host integration | Configure one of the sources below | Host credentials, using [source precedence](auth-proxy.md#credential-sources) |

**For legacy host integration**

| Agent | Sources |
|---|---|
| Codex | `OPENAI_API_KEY`, saved API key, `~/.codex/auth.json` |
| Claude Code | `ANTHROPIC_API_KEY`, `CLAUDE_CODE_OAUTH_TOKEN`, `~/.claude/.credentials.json`, saved API key |

- Login files: place under the invoking user's HOME.
- File access: the proxy uses that user's UID/GID; `0600` files need no permission changes.
- Workers receive placeholders, never real API keys or OAuth access/refresh tokens.
- Claude details: [API-key and subscription setup](claude-code.md).

## 3. Prepare images and start the proxies

**Run from the repository root as your normal account.**

```bash
oaiso init
# For Claude Code:
oaiso init --agent claude
```

| `init` action | Behavior or limit |
|---|---|
| Diagnose host and find credentials | Does not print credential contents or rewrite settings |
| Prepare worker image | Builds missing images and checks compatibility |
| Start proxies | Building a missing proxy image requires the Go source |
| Choose agent | Explicit `--agent`, then configured default; a shell default prepares Codex |
| Existing proxy with different authentication | Running workers prevent a forced switch |
| Outside this check | Live provider authentication, connectivity and inference |

- Missing source: run `oaiso proxy build` once at the repository root.
- Once ready: return to the project directory.
- Make Codex the default: `oaiso default codex`; add `--local` for this project only.

### Prepare each component separately

```bash
oaiso proxy build
oaiso proxy run
oaiso build
```

| Component | Purpose or endpoint |
|---|---|
| `codex-auth-proxy` | Authenticated AI requests |
| `codex-http-proxy` | General HTTP/CONNECT; no real credentials |
| Admin API | Host loopback `127.0.0.1:18081` |
| Worker-facing API | Not published to the host |

- `proxy run` creates the required network.
- Details: [auth proxy](auth-proxy.md).

## 4. Start a worker

- Run from the project directory.
- The project is shared at `/workspace`.

```bash
oaiso codex
# Or:
oaiso run --agent claude

# Omit --agent for a shell with proxy authentication configured.
oaiso run
oaiso run -- ls -la
```

| Invocation | Result |
|---|---|
| `oaiso codex` | Start Codex |
| `oaiso run --agent claude` | Start Claude Code |
| `oaiso run` | Configured agent; built-in default is an authenticated shell |
| `oaiso run -- COMMAND` | Run a command once |
| Failed isolation check | Startup stops; no automatic fallback to runc |

### Share an environment with a team

1. Run `oaiso project init` at the repository root.
2. Commit `codex-dock.toml`.
3. Point `[build]` to a project Dockerfile for extra tools.
4. After cloning, prepare the host and credentials.
5. Run `oaiso project check`, then `oaiso init`.

[Project settings](configuration.md#project-settings)

## 5. Run in the background

```bash
oaiso run --agent codex --name add-tests \
  --task "Add unit tests" --approval-mode full-auto \
  --worktree --branch add-tests --new-branch -d

oaiso ps -a
oaiso logs add-tests --follow
```

- `full-auto` disables agent approval prompts.
- Mount permissions still determine which shared files are writable.

## 6. Stop, restart and remove

```bash
oaiso stop add-tests
oaiso start add-tests
oaiso rm add-tests --force
```

| Operation or condition | Result |
|---|---|
| Stop | Revoke the token; retain the container and snapshot |
| Restart | Recheck isolation and restore auth for the same worker ID/workspace |
| Remove | Clean up managed worktrees and networks; save results first |
| Foreground exit | Remove the worker unless `--keep` is set |
| Shared proxies | Keep running after workers exit |

## Next steps

| Task | Guide |
|---|---|
| Updates and cleanup | [Operations](../operations.md) (Japanese) |
| Diagnose errors | [Troubleshooting](../troubleshooting.md) (Japanese) |
| Configure Claude authentication | [API-key and subscription setup](claude-code.md) |
