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

> All available keys are listed in `configs/config.toml.example`.

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

### `default_template`

Default sandbox image template. Applies to both `build` and `run`.

```toml
default_template = "plain"
```

| Item | Value |
|---|---|
| Type | string |
| Default | unset (no template) |
| Corresponding flag | `build --template`, `run --template`/`-T` |
| Allowed values | `"plain"`, `"pwn"`, etc. (run `codex-dock build --template list` to see all) |

When a template is set, the corresponding tagged image is used (e.g. `"pwn"` → `codex-dock:pwn`).
Section-specific settings (`build.template`, `run.template`) take precedence over this global default.

---

### `auto_build`

Whether `codex-dock run` rebuilds an existing template-derived image when the
template's Dockerfile has changed on disk since the image was built. Building an
image that's simply missing always happens regardless of this setting.

```toml
auto_build = true
```

| Item | Value |
|---|---|
| Type | boolean |
| Default | `true` (when unset) |
| Used by | `codex-dock run`'s auto-build check |

The check is a straightforward comparison between the template Dockerfile's
modification time and the built image's creation timestamp (`docker inspect`'s
`Created`). It only has an effect for templates whose Dockerfile exists as a
real file on disk — user templates under `~/.config/codex-dock/templates/`, or
built-in templates copied there via `make install-templates`. Built-in templates
that remain purely embedded in the binary have no file to compare against, so
this setting has no effect on them (missing-image auto-build still applies).

`run.auto_build` takes precedence over this global default when set.

---

### `default_shell_type`

Which interactive shell is launched inside the sandbox: the no-agent shell,
and `--shell` debug mode.

```toml
default_shell_type = "bash"
```

| Item | Value |
|---|---|
| Type | string |
| Default | `"bash"` (when unset) |
| Corresponding flag | `run --shell-type` |
| Allowed values | `"bash"`, `"zsh"` |

Using `"zsh"` requires zsh to be installed in the target image (the built-in
`plain` template includes it, and `pwn` inherits it, so both work out of the
box). `run.shell_type` takes precedence over this global default when set.

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

### `run.template`

Default template for `codex-dock run`.

```toml
[run]
template = "pwn"
```

| Item | Value |
|---|---|
| Type | string |
| Default | unset (falls back to `default_template`) |
| Corresponding flag | `run --image` (indirect: resolved to the template's tag) |

> `run.template` takes precedence over `default_template`.
> An explicit `--image` flag always takes precedence over template settings.

---

### `run.auto_build`

`run`-specific override for `auto_build`.

```toml
[run]
auto_build = false
```

| Item | Value |
|---|---|
| Type | boolean |
| Default | unset (falls back to the global `auto_build`) |
| Used by | `codex-dock run`'s auto-build check |

> `run.auto_build` takes precedence over the global `auto_build` when set.

---

### `run.shell_type`

`run`-specific override for `default_shell_type`.

```toml
[run]
shell_type = "zsh"
```

| Item | Value |
|---|---|
| Type | string |
| Default | unset (falls back to the global `default_shell_type`) |
| Corresponding flag | `run --shell-type` |
| Allowed values | `"bash"`, `"zsh"` |

> `run.shell_type` takes precedence over the global `default_shell_type` when set.
> An explicit `--shell-type` flag always takes precedence over the config file.

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

### `run.mounts`

Default value for `codex-dock run --mount` (an array; repeatable). Ignored
when `--mount` is passed on the command line — the flag always wins.

```toml
[run]
mounts = ["/home/user/datasets:/data:ro", "./cache:/cache"]
```

| Item | Value |
|---|---|
| Type | array of strings |
| Default | none (no extra mounts) |
| Corresponding flag | `run --mount` |
| Element format | `<host-path>:<container-path>[:ro\|rw]` (defaults to `rw`) |

`<container-path>` must be absolute; `/workspace` (reserved for `--project`) cannot be used.

---

### `[build]` section — `codex-dock build` defaults

#### `build.template`

Default value for `codex-dock build --template`.

```toml
[build]
template = "pwn"
```

| Item | Value |
|---|---|
| Type | string |
| Default | unset (falls back to `default_template`) |
| Corresponding flag | `build --template`, `-T` |

> `build.template` takes precedence over `default_template`.
> An explicit `--template` flag always takes precedence over the config file.

#### `build.tag`

Default value for `codex-dock build --tag`.

```toml
[build]
tag = "codex-dock:latest"
```

| Item | Value |
|---|---|
| Type | string |
| Default | `"codex-dock:latest"` |
| Corresponding flag | `build --tag`, `-t` |

---

### `[proxy]` section — Auth Proxy / router defaults

> The old `[firewall]` section (`allow_hosts` / `block_hosts` / `sudo`) has been
> removed. Network isolation is fully handled by Docker; there are no allow/block
> rules to configure.

```toml
[proxy]
# Auth Proxy URL reachable from workers (Docker DNS name + data-plane port)
container_url = "http://codex-auth-proxy:18080"
# Forward-proxy domain allowlist (subdomains included; omit to allow all)
forward_allow_domains = ["github.com", "registry.npmjs.org", "pypi.org"]
```

#### `proxy.container_url`

| Item | Value |
|---|---|
| Type | string |
| Default | `"http://codex-auth-proxy:18080"` |
| Corresponding flags | `run --proxy-container-url` |

The Auth Proxy data-plane URL workers reach over Docker DNS. The legacy key
`firewall.proxy_container_url` is still honored for now.

#### `proxy.forward_allow_domains`

| Item | Value |
|---|---|
| Type | array of strings |
| Default | unset (all domains allowed) |
| Corresponding flags | `proxy run --forward-allow-domain` (repeatable) |

Restricts the HTTP CONNECT forward proxy to the listed domains (and their
subdomains); everything else returns 403.

> **Note**: to disable general egress for a specific worker, use
> `codex-dock run --no-internet` (it omits `HTTP(S)_PROXY`, leaving only the
> proxy's API reverse routes reachable).

---

### `network_name`

The egress (proxy) network name.

```toml
network_name = "dock-net-proxy"
```

| Item | Value |
|---|---|
| Type | string |
| Default | `"dock-net-proxy"` |
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

# Egress (proxy) network name
network_name = "dock-net-proxy"

# Default template (if unset, no template is used)
# default_template = "plain"

# Rebuild on run if the template's Dockerfile changed (if unset, true)
# auto_build = true

# Default interactive shell (if unset, bash): "bash" | "zsh"
# default_shell_type = "bash"

# Verbose logging (normally false)
verbose = false

# Debug logging (only for development/troubleshooting)
debug = false

[run]
# run subcommand default image (if unset, default_image is used)
image = "codex-dock:latest"

# run subcommand template (if unset, default_template is used)
# template = "pwn"

# run subcommand token TTL (if unset, default_token_ttl is used)
token_ttl = 3600

# run subcommand auto_build override (if unset, the global auto_build is used)
# auto_build = true

# run subcommand shell_type override (if unset, the global default_shell_type is used)
# shell_type = "zsh"

# run subcommand default user
user = "current"

# run subcommand default approval mode
approval_mode = "suggest"

[build]
# build subcommand template (if unset, default_template is used)
# template = "pwn"

# build subcommand default tag
# tag = "codex-dock:latest"

[proxy]
# Auth Proxy URL reachable from workers (--proxy-container-url)
container_url = "http://codex-auth-proxy:18080"

# Forward-proxy domain allowlist (--forward-allow-domain)
forward_allow_domains = ["github.com", "registry.npmjs.org", "pypi.org"]
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
