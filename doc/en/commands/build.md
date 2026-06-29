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
| `--template` | `-T` | | Template name (`plain`, `pwn`, etc.). Use `--template list` to see available templates |

> **Note**: `--template` and `--dockerfile` (`-f`) are mutually exclusive.

Dockerfile search order (when `--template` is not used): current dir → `docker/` subdir → `~/.config/codex-dock/` (auto-written if missing).

> **Optional**: `codex-dock run` auto-builds the image if it doesn't exist.

---

## Templates

Use `--template` to build specialized images for specific workflows. Template Dockerfiles are embedded in the binary, so no source checkout is required.

### Available Templates

| Template | Tag | Description |
|---|---|---|
| `plain` | `codex-dock:latest` | Minimal setup (Codex CLI + Claude Code + basic tools). Same as the default Dockerfile |
| `pwn` | `codex-dock:pwn` | CTF / binary exploitation. Includes pwntools, ptrlib, ropper, pwndbg, radare2, gdb, nasm, strace, ltrace, binwalk, etc. |

Derived templates (e.g. `pwn`) extend the base image (`codex-dock:latest`) via `FROM`. If the base image hasn't been built yet, it is built automatically first.

### Template Validation

Before building, templates are statically validated to ensure they include essential tools:

- `@openai/codex` (Codex CLI) installation
- `@anthropic-ai/claude-code` (Claude Code) installation
- Basic tools (`git`, `curl`)
- Non-root user creation and `USER` switch
- `entrypoint.sh` copy

Derived templates (`FROM codex-dock:*`) trust the base image and skip these checks.

### Examples

```bash
# List available templates
codex-dock build --template list

# Build the plain template (same as default)
codex-dock build --template plain

# Build the pwn template (tagged codex-dock:pwn)
codex-dock build --template pwn

# Build pwn template with a custom tag
codex-dock build --template pwn --tag my-pwn:v1

# Use a template image with codex-dock run
codex-dock run --image codex-dock:pwn
```

---

## Related Documentation

- [`codex-dock run`](run.md)
- [Quick Start](../getting-started.md)
