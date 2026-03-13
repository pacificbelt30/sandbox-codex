# Configuration Reference

> [日本語](../configuration.md) | **English**

codex-dock is configured via `~/.config/codex-dock/config.toml`.
It uses TOML format and lets you change default values for command-line flags.

If you don't have a config file yet, run this first:

```bash
make install-config
```

---

## Config File Location

| Location | Description |
|---|---|
| `~/.config/codex-dock/config.toml` | Default config file |
| `--config <path>` | Custom path |

---

## Priority Order

Configuration values are resolved in the following priority order (higher takes precedence):

```
1. Command-line flags  (highest priority)
       │
       ▼
2. Environment variables (CODEX_DOCK_*)
       │
       ▼
3. config.toml
       │
       ▼
4. Built-in defaults  (lowest priority)
```

---

## Configuration Settings

> All available keys are listed in [`configs/config.toml.example`](../../configs/config.toml.example).

### `default_image`

Default Docker image to use for sandbox containers.

```toml
default_image = "codex-dock:latest"
```

| Item | Value |
|---|---|
| Type | string |
| Default | `"codex-dock:latest"` |
| Corresponding flag | `--image`, `-i` |
| Environment variable | `CODEX_DOCK_DEFAULT_IMAGE` |

**Example:**
```toml
default_image = "my-custom-sandbox:v2"
```

---

### `default_token_ttl`

Expiration time (in seconds) for short-lived tokens issued by Auth Proxy.

```toml
default_token_ttl = 3600
```

| Item | Value |
|---|---|
| Type | integer |
| Default | `3600` (1 hour) |
| Corresponding flag | `--token-ttl` |
| Environment variable | `CODEX_DOCK_DEFAULT_TOKEN_TTL` |

**Examples:**
```toml
# Shorter TTL (more secure)
default_token_ttl = 1800

# Longer TTL (for long-running tasks)
default_token_ttl = 28800
```

> **Security**: Shorter TTL reduces risk if a token is leaked.
> 1–2 hours is recommended for typical usage.

---

### `proxy_image`

Default Docker image for the Auth Proxy container.

```toml
proxy_image = "codex-dock-proxy:latest"
```

| Item | Value |
|---|---|
| Type | string |
| Default | `"codex-dock-proxy:latest"` |
| Used by | `proxy build`, `proxy run` |
| Environment variable | `CODEX_DOCK_PROXY_IMAGE` |

---

### `run.image`

Default value for `codex-dock run --image`.

```toml
[run]
image = "codex-dock:latest"
```

| Item | Value |
|---|---|
| Type | string |
| Default | unset (falls back to `default_image`) |
| Corresponding flag | `run --image`, `-i` |

> `run.image` takes precedence over `default_image`.

---

### `run.token_ttl`

Default value for `codex-dock run --token-ttl`.

```toml
[run]
token_ttl = 3600
```

| Item | Value |
|---|---|
| Type | integer |
| Default | unset (falls back to `default_token_ttl`) |
| Corresponding flag | `run --token-ttl` |

> `run.token_ttl` takes precedence over `default_token_ttl`.

---

### `run.user`

Default value for `codex-dock run --user`.

```toml
[run]
user = "current"
```

| Item | Value |
|---|---|
| Type | string |
| Default | `"current"` |
| Corresponding flag | `run --user` |
| Recommended values | `current`, `codex`, `dir`, `uid[:gid]` |

`codex` matches the historical default behavior (run as container `codex` user `1001:1001`).

---

### `run.approval_mode`

Default value for `codex-dock run --approval-mode`.

```toml
[run]
approval_mode = "suggest"
```

| Item | Value |
|---|---|
| Type | string |
| Default | `"suggest"` |
| Corresponding flag | `run --approval-mode` |
| Allowed values | `suggest`, `auto-edit`, `full-auto`, `danger` |

---

### `network_name`

Docker network name to use.

```toml
network_name = "dock-net"
```

| Item | Value |
|---|---|
| Type | string |
| Default | `"dock-net"` |
| Environment variable | `CODEX_DOCK_NETWORK_NAME` |

> Normally no change needed. Use this to separate multiple codex-dock environments.

---

### `verbose`

Whether to output detailed logs by default.

```toml
verbose = false
```

| Item | Value |
|---|---|
| Type | boolean |
| Default | `false` |
| Corresponding flag | `--verbose`, `-v` |
| Environment variable | `CODEX_DOCK_VERBOSE` |

Additional information shown in verbose mode:
- Auth Proxy listen address
- Token issuance, revocation, and expiry
- Credential delivery to containers
- Container creation details

---

### `debug`

Whether to output debug logs by default.

```toml
debug = false
```

| Item | Value |
|---|---|
| Type | boolean |
| Default | `false` |
| Corresponding flag | `--debug` |
| Environment variable | `CODEX_DOCK_DEBUG` |

Additional information shown in debug mode:
- Issued token details (TTL, container name)

---

## Example Config File

```toml
# ~/.config/codex-dock/config.toml
# codex-dock configuration file

# Default image to use
default_image = "codex-dock:latest"

# Token TTL (seconds): 1 hour
default_token_ttl = 3600

# Docker network name
network_name = "dock-net"

# Verbose logging (normally false)
verbose = false

# Debug logging (only for development/troubleshooting)
debug = false

[run]
# run subcommand default image (if unset, default_image is used)
image = "codex-dock:latest"

# run subcommand token TTL (if unset, default_token_ttl is used)
token_ttl = 3600

# run subcommand default user
user = "current"

# run subcommand default approval mode
approval_mode = "suggest"
```

---

## Auth File Locations

Auth-related files used by codex-dock:

| File | Location | Description |
|---|---|---|
| API key | `~/.config/codex-dock/apikey` | Saved by `codex-dock auth set` |
| OAuth credentials | `~/.codex/auth.json` | Generated by Codex CLI |
| Token rotation marker | `~/.config/codex-dock/.rotate` | Updated by `auth rotate` |

### Format of `~/.config/codex-dock/apikey`

```json
{"key": "sk-..."}
```

Permissions: `0600` (owner read/write only)

### Format of `~/.codex/auth.json`

**API key mode:**
```json
{
  "OPENAI_API_KEY": "sk-..."
}
```

**OAuth mode (ChatGPT subscription):**
```json
{
  "access_token": "eyJhbGci...",
  "refresh_token": "rt-...",
  "expires_at": 1735689600,
  "token_type": "Bearer"
}
```

> When the `refresh_token` field is present, OAuth mode is activated automatically.

---

## Container Environment Variables

Environment variables injected into containers by `codex-dock run` (for reference):

| Variable | Content | Configurable |
|---|---|---|
| `CODEX_SANDBOX` | Always `"1"` | No |
| `CODEX_AUTH_PROXY_URL` | Auth Proxy URL | No (auto-set) |
| `CODEX_TOKEN` | Short-lived token | No (auto-issued) |
| `CODEX_TASK` | Task prompt | Via `--task` |
| `CODEX_MODEL` | Model name | Via `--model` |
| `CODEX_APPROVAL_MODE` | Approval mode (`auto-edit` / `full-auto` / `danger`) | Via `--approval-mode` |
| `CODEX_INSTALL_SCRIPT` | Package install script | Via `--pkg` |
| `CODEX_AGENTS_MD` | Path to AGENTS.md | Via `--agents-md` |
