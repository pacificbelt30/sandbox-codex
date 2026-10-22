# `oaiso auth` — Authentication Management

> [日本語](../../commands/auth.md) | **English**
>
> [← Command Reference](../commands.md)

| Goal | Command or setting |
|---|---|
| Store a product-specific API key | `auth profile set NAME --provider openai\|anthropic` |
| Import OAuth credentials | `auth profile import NAME --credentials-stdin --format codex\|claude\|credentials` |
| Inspect redacted metadata | `auth profile show NAME` / `auth profile list` |
| Select a profile | `--auth-profile NAME` or global `auth_profile` |
| Keep legacy host integration | Leave profiles unselected; use `auth show` / `auth set` |

## Independent profiles

### Behavior and storage

| Item | Contract |
|---|---|
| Separation | Never automatically reuse host CLI logins or authentication variables |
| Missing profile or unsafe permissions | Fail; never fall back to host credentials |
| Example file | `~/.config/codex-dock/profiles/work/work.json` |
| Root and profile directories | `0700` |
| JSON and lock files | `0600`, plaintext |
| Override the root | `--auth-profile-dir` or trusted global `auth_profile_dir` |
| WSL | Private Linux-side storage, outside shared/synced folders |
| Input | No keys in arguments; noninteractive input requires explicit stdin |

### Register an API key

```bash
oaiso auth profile set work --provider openai  # hidden TTY input
oaiso auth profile set claude-work --provider anthropic
oaiso auth profile show work
oaiso auth profile list
```

- Interactive: hidden TTY input.
- Noninteractive: `--key-stdin` with a protected input source.
- Live API-key validation: not implemented.

### Import OAuth

```bash
oaiso auth profile import work-oauth --format codex --credentials-stdin < /private/product-login/auth.json
oaiso auth profile import claude-oauth --format claude --credentials-stdin < /private/product-login/.credentials.json
```

| `--format` | Input |
|---|---|
| `codex` | Native `tokens` and `last_refresh` |
| `claude` | `claudeAiOauth` |
| `credentials` (default) | `credentialstore.Credentials` JSON; can contain both providers |

- `--credentials-stdin`: required.
- Source files: never automatically discovered or deleted.
- Automated login-only Kata: not implemented.

### Create admin authority and start the proxy

- Store admin authority in a separate `0600` file.
- Pass its path, keeping the secret value out of process arguments.
- The generator below refuses to overwrite an existing file.

```bash
python3 - <<'PY'
import os, pathlib, secrets
path = pathlib.Path.home() / '.config/codex-dock/proxy-admin-secret'
with os.fdopen(os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600), 'w') as f:
    f.write(secrets.token_hex(32) + '\n')
PY
oaiso --auth-profile work proxy run --admin-secret-file "$HOME/.config/codex-dock/proxy-admin-secret"
oaiso --auth-profile work run --agent codex --proxy-admin-secret-file "$HOME/.config/codex-dock/proxy-admin-secret"
```

### Fix the selection for daily operations

**Location: trusted global `~/.config/codex-dock/config.toml`.**

```toml
auth_profile = "work"
[proxy]
admin_secret_file = "/home/user/.config/codex-dock/proxy-admin-secret"
```

| Command | Selection or output |
|---|---|
| `init`, `run`, `start`, TUI | Share the global settings above |
| `oaiso run` | Also accepts explicit profile flags |
| `oaiso codex` / `oaiso claude` | Forward arguments to the agent; select profiles through global settings |
| `auth show` | Redacted metadata for the selected profile |

- Never put authentication configuration in the project's `codex-dock.toml`.

### Refresh, switching and sharing

| Operation or boundary | Behavior |
|---|---|
| Creation/import | Create-only; never overwrite an existing profile |
| Normal OAuth refresh | CAS-save to the same Store; preserve connection generation |
| Re-login | Import under a new profile name |
| Retained workers | No reauthorization under another profile or login generation |
| Auth proxy | Only the selected directory is mounted read-write for refresh |
| Workers and egress | Never receive credential storage |
| Workspaces and extra mounts | Must exclude credential storage and admin-secret files |

**Switch after re-login**

1. Create/import a new profile name.
2. Stop and remove old workers.
3. Explicitly recreate the auth proxy.
4. Select the new profile and create new workers.

- The following legacy commands apply without profile selection.
- Profile mode rejects legacy `auth set` / `auth rotate`.

## `auth show`

```bash
oaiso auth show
```

- Shows the current authentication source.
- Never displays actual keys or tokens.

## `auth set`

```bash
export OPENAI_API_KEY=sk-...
oaiso auth set
```

| Item | Value |
|---|---|
| Source | `OPENAI_API_KEY` |
| Destination | `~/.config/codex-dock/apikey` |
| Permissions | `0600` |
| Condition | No independent profile selected |

## `auth rotate`

```bash
oaiso auth rotate
```

| Goal | Operation |
|---|---|
| Write a local update marker | `auth rotate`; does not immediately revoke a running proxy's tokens |
| Revoke immediately | Stop the worker or use `/admin/revoke` |
| Update the proxy | [Operations](../../operations.md) (Japanese) |

## Related Documentation

| Topic | Guide |
|---|---|
| Source precedence and credential protection | [Auth proxy](../auth-proxy.md) |
| Configuration values and files | [Configuration](../configuration.md) |
| Initial setup | [Quick start](../getting-started.md) |
