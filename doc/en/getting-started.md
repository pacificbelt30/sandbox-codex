# Quick Start

> [日本語](../getting-started.md) | **English**

This guide walks you through installing codex-dock and launching your first container.

---

## Prerequisites

| Requirement | Version | How to Check |
|---|---|---|
| Go | 1.22 or later | `go version` |
| Docker Engine | 20.10 or later | `docker version` |
| git | any | required for worktree feature |

---

## Installation

### Using Makefile (Recommended)

The Makefile lets you build the binary, install it, place the default config file, and build the Docker image all at once.

```bash
# Clone the repository
git clone https://github.com/pacificbelt30/codex-dock.git
cd codex-dock

# Build the binary and the codex-dock:latest image
make all

# Install the binary to /usr/local/bin and place the default config
sudo make install-all
```

The above steps complete the following:

| What | Where |
|---|---|
| Binary | `/usr/local/bin/codex-dock` |
| Default config | `~/.config/codex-dock/config.toml` |
| Sandbox image | `codex-dock:latest` (Docker) |

> **Note**: `sudo make install-all` detects the invoking user's home directory via `$SUDO_USER`, so the config is placed under your home (`~/.config/codex-dock/`), not `/root/.config/`.

To install to a custom location:

```bash
sudo make install PREFIX=/opt/codex-dock
```

You can also run targets individually:

```bash
make build          # build binary only
make docker         # build Docker image only
make install-config # place config only (skipped if already exists)
make uninstall      # remove installed binary
make clean          # remove build artifacts
```

### Build from Source (Manual)

```bash
# Clone the repository
git clone https://github.com/pacificbelt30/codex-dock.git
cd codex-dock

# Build binary
go build -o codex-dock .

# Add to PATH (optional)
sudo mv codex-dock /usr/local/bin/
```

---

## Step 1: Build the Sandbox Image

Build the image for running Codex CLI inside containers.

```bash
codex-dock build
```

**What happens internally:**
- Auto-detects Dockerfile (current directory → `~/.config/codex-dock/`)
- Node.js 22 base + Codex CLI (`@openai/codex`) installed
- Creates non-root user `codex` (uid:1001)
- Default tag: `codex-dock:latest`

> **Optional**: If the image doesn't exist when you run `codex-dock run`, it will be built automatically.
> On first use, `codex-dock run` alone is enough.

To use a custom Dockerfile, specify it with `-f`:

```bash
codex-dock build -f ./my/Dockerfile --tag my-codex:v1
```

---

## Step 2: Configure Authentication

### Using an API Key

```bash
export OPENAI_API_KEY=sk-...
codex-dock auth set
```

Or simply set the environment variable each time you run:

```bash
export OPENAI_API_KEY=sk-...
codex-dock run
```

### Using ChatGPT Subscription (OAuth)

First, log in with the standard Codex CLI:

```bash
codex login
```

Once `~/.codex/auth.json` is generated, codex-dock will detect it automatically.

```bash
# Check auth status
codex-dock auth show
# Auth source: ~/.codex/auth.json (OAuth/ChatGPT subscription)
```

---

## Step 3: Launch Your First Container

```bash
# Mount current directory to /workspace and start Codex
codex-dock run
```

**What happens automatically:**

```
1. Create dock-net (first time only)
       │
       ▼
2. Start Auth Proxy
   (127.0.0.1:random port)
       │
       ▼
3. Issue short-lived token
       │
       ▼
4. Create and start container
   - Network: dock-net
   - Mount:   ./  →  /workspace
   - env:     CODEX_TOKEN=cdx-xxxx
       │
       ▼
5. entrypoint.sh runs inside container
   → Fetches credentials from Auth Proxy
   → Launches Codex CLI
```

---

## Common Patterns

### Run a Task Automatically

```bash
codex-dock run \
  --task "Add unit tests to src/auth.go" \
  --full-auto \
  --detach
```

Check logs when complete:

```bash
codex-dock ps --all
codex-dock logs codex-brave-atlas --tail 50
```

### Safe Branch Work with git worktree

Work on a new branch without modifying the original repository.

```bash
codex-dock run \
  --worktree \
  --branch feature-auth \
  --new-branch \
  --task "Refactor the auth module"
```

The worktree is automatically deleted when the container exits.

### Run Multiple Tasks in Parallel

```bash
# Run on 3 branches in parallel
codex-dock run --parallel 3 --worktree --detach
```

Monitor all workers with TUI:

```bash
codex-dock ui
```

### Run with Additional Packages

```bash
codex-dock run --pkg "apt:libssl-dev" --pkg "pip:cryptography"
```

Or manage via a `packages.dock` file:

```bash
# Create packages.dock
cat > packages.dock << 'EOF'
apt:libssl-dev
pip:cryptography
pip:pytest
npm:typescript
EOF

# Auto-detected and installed
codex-dock run
```

---

## Check Network Status

```bash
# Check dock-net status
codex-dock network status

# Check running workers
codex-dock ps
```

---

## Cleanup

```bash
# Stop all containers
codex-dock stop --all

# Remove stopped containers
codex-dock rm <container-name>

# Remove dock-net (if needed)
codex-dock network rm
```

---

## Troubleshooting

### Container Fails to Start

```bash
# Run with detailed logging
codex-dock run --verbose --debug
```

Common causes:
- Docker is not running → check with `docker ps`
- Image build failed → check with `codex-dock build --verbose`
- API key not set → check with `codex-dock auth show`

### Authentication Error

```bash
# Check auth status
codex-dock auth show

# Reset API key
export OPENAI_API_KEY=sk-...
codex-dock auth set
```

### Network Error

```bash
# Recreate dock-net
codex-dock network rm
codex-dock network create
```

### View Logs

```bash
# Real-time container logs
codex-dock logs <container-name> --follow

# Check all worker status
codex-dock ps --all
```

---

## Next Steps

- [Architecture Overview](architecture.md) — Understand the system as a whole
- [Auth Proxy Specification](auth-proxy.md) — Learn how authentication works
- [Network Specification](network.md) — Details of network isolation
- [Command Reference](commands.md) — All commands and options
- [Configuration Reference](configuration.md) — config.toml settings
