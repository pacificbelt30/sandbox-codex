# `codex-dock proxy` — Auth Proxy Management

> [日本語](../../commands/proxy.md) | **English**
>
> [← Command Reference](../commands.md)

Build, start, stop, and remove the Auth Proxy container.

---

## Subcommands

| Subcommand | Description |
|---|---|
| [`proxy build`](#proxy-build) | Build Auth Proxy image |
| [`proxy run`](#proxy-run) | Start Auth Proxy container |
| [`proxy serve`](#proxy-serve) | Start as a local process |
| [`proxy stop`](#proxy-stop) | Stop Auth Proxy container |
| [`proxy rm`](#proxy-rm) | Remove Auth Proxy container |

---

## `proxy build`

```bash
codex-dock proxy build [OPTIONS]
```

| Option | Short | Default | Description |
|---|---|---|---|
| `--tag` | `-t` | `proxy_image` (from config) | Image tag |
| `--dockerfile` | `-f` | (auto-detected) | Path to auth-proxy.Dockerfile |

Dockerfile search order: current dir → `docker/` subdir → `~/.config/codex-dock/` (auto-written if missing).

---

## `proxy run`

```bash
codex-dock proxy run [OPTIONS]
```

| Option | Short | Default | Description |
|---|---|---|---|
| `--name` | | `codex-dock-proxy` | Container name |
| `--port` | `-p` | `18080` | Host port |
| `--admin-secret` | | | Secret for `/admin/*` endpoints |

Automatically binds all detected auth sources to the container:

| Auth Type | Host Source | How Passed |
|---|---|---|
| API key (env) | `OPENAI_API_KEY` | `-e OPENAI_API_KEY=<value>` |
| API key (saved) | `~/.config/codex-dock/apikey` | bind-mount (read-only) |
| OAuth / ChatGPT | `~/.codex/auth.json` | bind-mount (read-only) |

---

## `proxy serve`

Run Auth Proxy as a local process (not in a container).

```bash
codex-dock proxy serve --listen 127.0.0.1:18080 [OPTIONS]
```

→ See [Using Auth Proxy Standalone](../proxy-standalone.md) for details.

---

## `proxy stop`

```bash
codex-dock proxy stop [--name codex-dock-proxy]
```

---

## `proxy rm`

```bash
codex-dock proxy rm [--name codex-dock-proxy] [--force]
```

---

## Related Documentation

- [Auth Proxy Specification](../auth-proxy.md)
- [API Endpoint Reference](../auth-proxy/endpoints.md)
- [Using Auth Proxy Standalone](../proxy-standalone.md)
