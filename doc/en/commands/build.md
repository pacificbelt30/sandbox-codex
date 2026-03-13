# `codex-dock build` — Build Sandbox Image

> [日本語](../../commands/build.md) | **English**
>
> [← Command Reference](../commands.md)

```bash
codex-dock build [OPTIONS]
```

| Option | Short | Default | Description |
|---|---|---|---|
| `--tag` | `-t` | `codex-dock:latest` | Image tag |
| `--dockerfile` | `-f` | (auto-detected) | Dockerfile path |

Dockerfile search order: current dir → `docker/` subdir → `~/.config/codex-dock/` (auto-written if missing).

> **Optional**: `codex-dock run` auto-builds the image if it doesn't exist.

---

## Related Documentation

- [`codex-dock run`](run.md)
- [Quick Start](../getting-started.md)
