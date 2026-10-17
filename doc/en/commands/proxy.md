# Manage proxies

[日本語](../../commands/proxy.md) | **English**

`codex-dock proxy` manages the auth and general egress proxies on containerd.
See [auth proxy](../auth-proxy.md) and [update procedure](../../operations.md#cliプロキシを更新する) (Japanese).

## Build

```bash
codex-dock proxy build
codex-dock proxy build --tag my-proxy:v1 -f ./docker/proxy/Dockerfile
```

`--tag/-t` overrides `proxy_image`; `--dockerfile/-f` selects the Dockerfile.
Standalone BuildKit imports OCI output into containerd. Build from the repository root: the current directory is the context and Go source is required.
The search order is `docker/proxy/Dockerfile`, legacy `auth-proxy.Dockerfile` / `docker/auth-proxy.Dockerfile`, then the config directory.

## Start

```bash
codex-dock proxy run
```

| Flag | Default | Purpose |
|---|---|---|
| `--name` | `codex-auth-proxy` | Auth container name |
| `--http-name` | `codex-http-proxy` | Egress container name |
| `--port`, `-p` | `18080` | Internal auth data port, not host-published |
| `--admin-port` | `18081` | Admin port published on host loopback |
| `--http-port` | `18082` | Internal egress port, not host-published |
| `--network` | `dock-net-proxy` | Shared egress network; created if missing |
| `--admin-secret` | Unset | Server secret; configure the same value in host clients |

Policy flags are `--forward-allow-domain`, `--forward-deny-domain`, `--forward-allow-cidr`, `--forward-deny-cidr`, `--forward-allow-port` and `--forward-deny-port`.
Domain/CIDR flags repeat; port flags repeat or accept comma-separated values.
See [policy rules](../../proxy-policy.md) (Japanese).

The auth proxy reads credentials as the caller's UID/GID through read-only binds under a private HOME.
Old layouts, policy mismatches and missing worker-auth capability are rejected rather than silently updating existing containers.

## Stop and remove

```bash
# Both default proxies
codex-dock proxy stop
codex-dock proxy rm

# One named proxy
codex-dock proxy stop --name codex-http-proxy
codex-dock proxy rm --name codex-http-proxy
```

Use `--name` for custom names. `proxy rm --force` removes a running container.
Finish worker tasks first; these proxies are shared services.

## Local process

The internal `proxy serve` command is available for development and standalone use:

```bash
codex-dock proxy serve --role auth \
  --listen 127.0.0.1:18080 --admin-listen 127.0.0.1:18081 \
  --admin-secret "$PROXY_ADMIN_SECRET"
```

Role `auth` serves AI/token/admin routes; `egress` forwards general traffic.
The default data listen address is `0.0.0.0:18080`, so set loopback explicitly for local use.
This command does not create Kata isolation or worker networks. See [standalone use](../proxy-standalone.md).
