# Configuration

[日本語](../configuration.md) | **English**

User settings live in `~/.config/codex-dock/config.toml`.
Use `codex-dock --config PATH COMMAND` to select another file.
`make install-config` installs the example without overwriting an existing file.

## Example

```toml
default_image = "codex-dock:latest"
proxy_image = "codex-dock-proxy:latest"

[run]
runtime = "kata"
user = "current"
approval_mode = "suggest"
token_ttl = 3600

[proxy]
container_url = "http://codex-auth-proxy:18080"
admin_url = "http://127.0.0.1:18081"
forward_allow_domains = ["github.com", "registry.npmjs.org"]
forward_allow_ports = [80, 443]
```

Use the [credential sources](auth-proxy.md#credential-sources) for API keys and OAuth files.
Explicit flags override config. Command-specific settings override global defaults.
Only the documented environment overrides should be relied on; arbitrary `CODEX_DOCK_*` names are not supported.

## Global settings

| Key | Default | Purpose |
|---|---|---|
| `default_image` | `codex-dock:latest` | Worker OCI image |
| `proxy_image` | `codex-dock-proxy:latest` | Image for both proxy roles |
| `default_template` | Unset | Template for run/build |
| `default_token_ttl` | `3600` | Token lifetime in seconds |
| `auto_build` | `true` | Rebuild changed on-disk templates |
| `default_shell_type` | Detect from host shell | bash/zsh |
| `verbose`, `debug` | `false` | CLI logging |

### `auto_build`

Compare the template Dockerfile mtime to the image creation timestamp.
Only on-disk templates are checked. Purely embedded templates have no file to compare.
Missing images are built regardless of this setting.

### `default_shell_type`

Without a setting, host `$SHELL` selects zsh when its basename is zsh, otherwise bash.
Debug `--shell` defaults to bash unless explicitly overridden.

## Run settings

Under `[run]`:

| Key | Default or inherited value | Flag |
|---|---|---|
| `runtime` | `kata` | `--runtime` |
| `kata_runtime` | `io.containerd.kata.v2` | `--kata-runtime` |
| `image` | `default_image` | `--image` |
| `template` | `default_template` | `--template` |
| `token_ttl` | `default_token_ttl` | `--token-ttl` |
| `approval_mode` | `suggest` | `--approval-mode` |
| `user` | `current` | `--user` |
| `shell_type` | `default_shell_type` | `--shell-type` |
| `auto_build` | `auto_build` | Config only |
| `mounts` | Empty | `--mount` |

Explicit `--image` wins over template image selection.
`runtime = "runc"` is an explicit compatibility mode on the same containerd node, without VM isolation.

### `run.mounts`

```toml
[run]
mounts = ["/home/user/datasets:/data:ro", "./cache:/cache:rw"]
```

Relative host paths resolve from the command's working directory.
An explicit `--mount` replaces the config array. `--read-only` controls the workspace; extra mounts have their own permissions.

The legacy `network_name` setting is not used by current network commands, which use `dock-net-proxy`.
Select a proxy network with `proxy run --network`.

## Build settings

Under `[build]`, `template` inherits `default_template` and `tag` sets the output image name.
Without an explicit tag, templates use their own tag; a plain build uses `codex-dock:latest`.
See [build](commands/build.md).

## Proxy settings

Under `[proxy]`:

| Key | Default | Used by |
|---|---|---|
| `container_url` | `http://codex-auth-proxy:18080` | `run --proxy-container-url` |
| `admin_url` | `http://127.0.0.1:18081` | `run --proxy-admin-url`, worker management |
| `admin_secret` | Unset | Admin client; also supply the server secret with `proxy run --admin-secret` |
| `forward_allow_domains`, `forward_deny_domains` | Empty | `--forward-allow-domain`, `--forward-deny-domain` |
| `forward_allow_cidrs`, `forward_deny_cidrs` | Empty | `--forward-allow-cidr`, `--forward-deny-cidr` |
| `forward_allow_ports`, `forward_deny_ports` | Empty | `--forward-allow-port`, `--forward-deny-port` |

The data URL is used by workers; the admin URL is used by the host CLI.
General egress uses `run --http-proxy-url`.
The legacy `firewall.proxy_container_url` key is still read, but use `[proxy]` for new config.

Rules apply only to general egress. Denials win and all nonempty allowlists must match.
Private address blocking cannot be overridden by an allowlist.
Recreate the proxy to apply changes. See [policy rules](../proxy-policy.md) (Japanese).

## Environment overrides

| Variable | Purpose |
|---|---|
| `CODEX_DOCK_RUNTIME` | Override `run.runtime` |
| `CODEX_DOCK_KATA_RUNTIME` | Override `run.kata_runtime` |
| `CODEX_DOCK_PROXY_ADMIN_SECRET` | Admin client secret |
| `CODEX_DOCK_KATA_NODE_SOCKET` | Absolute node socket path |
| `CODEX_DOCK_STATE_DIR` | Private absolute path for cleanup/retry records |
| `XDG_STATE_HOME` | State root when no dedicated state directory is set |
| `BUILDKIT_HOST` | Builder address; default `unix:///run/codex-dock-buildkit/buildkitd.sock` |
| HTTP/HTTPS/NO/ALL proxy variables, upper/lowercase | BuildKit build args |
| `CODEX_PROXY_CERT` | CA file passed as BuildKit `proxy_ca` secret |
| `OPENAI_API_KEY`, `ANTHROPIC_API_KEY` | Host proxy credentials |

Runtime overrides take precedence over config, below explicit flags.
`DOCKER_HOST` and Docker contexts are not used.

## Host service configuration

Host services are configured separately from user settings:
`node serve --help`, `/etc/codex-dock/containerd.toml`, `kata.toml`, `kata-assets.json` and `buildkit.toml`.
`[kata].node_socket` configures the verification client. If using a custom path, set `CODEX_DOCK_KATA_NODE_SOCKET` to the same value for the engine client.
See [host setup](../kata.md) (Japanese).

## Credential File Locations

| Path | Contents |
|---|---|
| `~/.config/codex-dock/apikey` | Saved OpenAI API key |
| `~/.codex/auth.json` | Codex login |
| `~/.config/codex-dock/anthropic-apikey` | Saved Anthropic API key |
| `~/.claude/.credentials.json` | Claude Code login |

Do not include their contents in logs or images. See [auth proxy](auth-proxy.md).
