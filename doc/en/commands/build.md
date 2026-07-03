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

### User Templates

Besides the templates built into the binary under `docker/templates/`, you can keep
**your own templates under `~/.config/codex-dock/templates/`.** No rebuild of
codex-dock is required — just drop in a Dockerfile and it becomes available via
`--template`. This is a good place for private or team-internal templates you don't
want to commit to the repo.

#### Where to put them

```
~/.config/codex-dock/templates/
└── <template-name>/
    └── Dockerfile
```

For example, to add a template named `mytool`:

```bash
mkdir -p ~/.config/codex-dock/templates/mytool
cp path/to/Dockerfile ~/.config/codex-dock/templates/mytool/Dockerfile

# verify it shows up (marked with [user])
codex-dock build --template list
#   Available templates:
#     plain        codex-dock:latest
#     pwn          codex-dock:pwn (extends codex-dock:latest)
#     mytool       codex-dock:mytool [user]
#
#   User templates directory: /home/you/.config/codex-dock/templates
#   Add <dir>/<name>/Dockerfile to add a template or override a built-in one.

# build it
codex-dock build --template mytool
```

#### Precedence (overriding a built-in template)

Placing a directory under `~/.config/codex-dock/templates/<name>/` with the same
name as a built-in template (e.g. `pwn`) makes the user version take precedence.
This lets you try out or customize a built-in template locally without editing
files in the repo.

| Precedence | Location |
|---|---|
| 1 (highest) | `~/.config/codex-dock/templates/<name>/Dockerfile` (user template) |
| 2 | `docker/templates/<name>/Dockerfile` (built-in, embedded in the binary) |

#### Validation

User templates go through **exactly the same static checks** (`template.Validate()`)
described in "Template Validation" below as built-in ones. A derived template
(`FROM codex-dock:*`) skips those checks just like a built-in derived template does.
So verifying and managing a user template is no different from a built-in one.

#### Seeding built-in templates at install time

Built-in templates (`plain`, `pwn`, etc.) only live embedded in the binary — by
default no file for them exists under `~/.config/codex-dock/templates/`. Running
`make install-templates` (or `make install-all`) copies `docker/sandbox/Dockerfile`
(`plain`) and each `docker/templates/<name>/Dockerfile` into that directory, so you
can view and edit them as regular files afterwards. Templates that already have a
file there are skipped and never overwritten.

```bash
make install-templates
#   Default template 'plain' installed at ~/.config/codex-dock/templates/plain/Dockerfile
#   Default template 'pwn'   installed at ~/.config/codex-dock/templates/pwn/Dockerfile
```

Once seeded, these are treated as user templates (`codex-dock build --template list`
marks them `[user]`) and take precedence over the binary-embedded originals, so
editing the file directly changes what gets built.

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
