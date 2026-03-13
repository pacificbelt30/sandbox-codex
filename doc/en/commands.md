# Command Reference

> [日本語](../commands.md) | **English**

## Global Options

Options available for all commands:

| Option | Short | Default | Description |
|---|---|---|---|
| `--verbose` | `-v` | `false` | Print detailed logs |
| `--debug` | | `false` | Print debug logs |
| `--config` | | `~/.config/codex-dock/config.toml` | Path to config file |

---

## `codex-dock run` — Launch Sandbox Container

Runs Codex CLI inside a Docker container. Auth Proxy and network isolation are configured automatically.

> **Auto image build**: If the image specified by `--image` does not exist locally, it will be built automatically using the same logic as `codex-dock build`.

```bash
codex-dock run [OPTIONS]
```

### Options

| Option | Short | Default | Description |
|---|---|---|---|
| `--image` | `-i` | `codex-dock:latest` | Docker image to use for the sandbox |
| `--pkg` | `-p` | | Additional packages (repeatable): `apt:<pkg>`, `pip:<pkg>`, `npm:<pkg>` |
| `--pkg-file` | | | Path to package definition file (`packages.dock`) |
| `--project` | `-d` | `.` (current dir) | Project directory to mount at `/workspace` |
| `--worktree` | `-w` | `false` | Use git worktree to isolate the container |
| `--branch` | `-b` | | Branch name to check out (requires `--worktree`) |
| `--new-branch` | `-B` | `false` | Create a new branch (requires `--worktree` and `--branch`) |
| `--name` | `-n` | auto-generated | Container name (auto-generates `codex-<adjective>-<noun>` if omitted) |
| `--task` | `-t` | | Initial task prompt to pass to Codex |
| `--approval-mode` | | `suggest` | Codex CLI approval mode (see below) |
| `--full-auto` | | `false` | **Deprecated**: use `--approval-mode full-auto` instead |
| `--model` | `-m` | | Model name to pass to Codex |
| `--read-only` | | `false` | Mount project directory as read-only |
| `--no-internet` | | `false` | Disable internet access inside container |
| `--token-ttl` | | `3600` | Auth Proxy token TTL (seconds) |
| `--agents-md` | | | Path to `AGENTS.md` file |
| `--detach` | `-D` | `false` | Run in background (no log output) |
| `--parallel` | `-P` | `1` | Number of parallel workers |
| `--user` | | `""` | User to run container process as (see below) |

### `--approval-mode` — Codex Approval Mode

The `--approval-mode` flag controls how Codex CLI handles approval of actions.
Designed with Docker container sandbox isolation in mind.

| Value | Codex CLI Flag | Behavior |
|---|---|---|
| `suggest` (default) | none | Prompts for approval on all actions (safest) |
| `auto-edit` | `--ask-for-approval unless-allow-listed` | Auto-applies file edits; prompts for command execution |
| `full-auto` | `--ask-for-approval never` | Never prompts for approval |
| `danger` | `--dangerously-bypass-approvals-and-sandbox` | Bypasses all approvals and sandbox restrictions |

> **About `danger` mode**: Disables the built-in Codex CLI sandbox, but the Docker container itself
> provides the isolation boundary (`--cap-drop ALL`, dedicated network, pids-limit, etc.).
> Host environment is unaffected because Docker is used.

```bash
# Default (interactive approval prompts)
codex-dock run --task "Write tests"

# Auto file edits, prompt for commands
codex-dock run --approval-mode auto-edit --task "Refactor"

# Fully automated (no prompts)
codex-dock run --approval-mode full-auto --task "Fix bug"

# Bypass all restrictions using Docker isolation
codex-dock run --approval-mode danger --task "Run build script"
```

### `--user` — Specify Container User

The `--user` flag lets you start the container process with an arbitrary uid:gid.
Used to ensure file permission compatibility with the host filesystem.

| Value | Behavior |
|---|---|
| `""` (omitted) | Image default user (`codex` uid:1001) |
| `current` | Auto-detect uid:gid of the user running `codex-dock` |
| `dir` | Auto-detect uid:gid of the `--project` directory owner |
| `uid` or `uid:gid` | Explicit specification (e.g., `1000`, `1000:1000`) |

> **Note**: When a custom user is specified, that user may not exist in the container's `/etc/passwd`.
> `codex-dock` automatically injects `HOME=/tmp`, so auth files and Codex CLI config will be written under `/tmp`.
> These are discarded automatically when the container exits.

### Examples

```bash
# Basic run (mount current directory)
codex-dock run

# Mount specific project directory
codex-dock run --project /path/to/myproject

# Run fully automated task
codex-dock run --task "Write unit tests" --approval-mode full-auto

# Work on feature branch using git worktree
codex-dock run --worktree --branch feature-auth --new-branch

# Launch 3 parallel workers (each on a separate branch)
codex-dock run --parallel 3 --worktree

# Install additional packages and run
codex-dock run --pkg "apt:libssl-dev" --pkg "pip:requests" --pkg "npm:lodash"

# Use a packages.dock file
codex-dock run --pkg-file ./packages.dock

# Run fully automated in background
codex-dock run --task "Refactor" --approval-mode full-auto --detach

# Run securely with read-only mount and no internet
codex-dock run --read-only --no-internet --task "Code review"

# Use a specific Docker image
codex-dock run --image my-custom-codex:v2

# Specify a custom model
codex-dock run --model "o3"

# Launch container with same uid:gid as the running user
codex-dock run --user current

# Launch container with uid:gid of project directory owner
codex-dock run --user dir --project /srv/myapp

# Specify uid:gid explicitly
codex-dock run --user 1000:1000
```

### Package Format

Package format for use with `--pkg` or `packages.dock` file:

```
apt:libssl-dev          # install via apt
pip:requests            # install via pip
npm:lodash              # install via npm
libssl-dev              # no prefix → treated as apt (default)
```

Example `packages.dock` file:

```
# Comments start with #
apt:libssl-dev
apt:postgresql-client
pip:requests
pip:numpy
npm:typescript
```

### Parallel Workers

Specifying `--parallel N` starts N containers simultaneously.

```bash
codex-dock run --parallel 3 --worktree --branch myfeature
```

Branches created automatically:
- `myfeature-1` (worker 1)
- `myfeature-2` (worker 2)
- `myfeature-3` (worker 3)

If `--branch` is not specified, `worker-1`, `worker-2`, `worker-3` are used.

---

## `codex-dock ps` — List Workers

Lists running containers.

```bash
codex-dock ps [OPTIONS]
```

| Option | Short | Default | Description |
|---|---|---|---|
| `--all` | `-a` | `false` | Include stopped containers |

**Example output:**

```
NAME                   STATUS    UPTIME    BRANCH         TASK
codex-brave-atlas      running   5m23s     feature-auth   Write unit tests
codex-calm-beacon      running   2m10s     main           (interactive)
```

---

## `codex-dock stop` — Stop Container

Stops running containers.

```bash
codex-dock stop [NAME|ID...] [OPTIONS]
```

| Option | Short | Default | Description |
|---|---|---|---|
| `--all` | `-a` | `false` | Stop all running containers |
| `--timeout` | | `10` | Wait time before force stop (seconds) |

**Examples:**

```bash
# Stop a specific container
codex-dock stop codex-brave-atlas

# Stop multiple containers
codex-dock stop codex-brave-atlas codex-calm-beacon

# Stop all containers
codex-dock stop --all
```

---

## `codex-dock rm` — Remove Container

Removes stopped containers.

```bash
codex-dock rm [NAME|ID...] [OPTIONS]
```

| Option | Short | Default | Description |
|---|---|---|---|
| `--force` | `-f` | `false` | Force remove even running containers |

**Examples:**

```bash
# Remove a stopped container
codex-dock rm codex-brave-atlas

# Force remove a running container
codex-dock rm --force codex-brave-atlas
```

---

## `codex-dock logs` — View Logs

Displays container logs.

```bash
codex-dock logs NAME|ID [OPTIONS]
```

| Option | Short | Default | Description |
|---|---|---|---|
| `--tail` | `-n` | `100` | Number of lines to show from the end |
| `--follow` | `-f` | `false` | Stream logs in real time |

**Examples:**

```bash
# Show last 100 lines
codex-dock logs codex-brave-atlas

# Stream logs in real time
codex-dock logs codex-brave-atlas --follow

# Show last 50 lines
codex-dock logs codex-brave-atlas --tail 50
```

---

## `codex-dock auth` — Auth Management

Manages API key and OAuth credentials.

### `auth show` — Check Auth Status

```bash
codex-dock auth show
```

Shows the current auth source (actual keys and tokens are never displayed).

**Example output (API key):**
```
Auth source: OPENAI_API_KEY env
Configured:  yes
```

**Example output (OAuth):**
```
Auth source: ~/.codex/auth.json (OAuth/ChatGPT subscription)
Configured:  yes
```

**Example output (not configured):**
```
Auth source: none
Configured:  no
```

### `auth set` — Save API Key

```bash
export OPENAI_API_KEY=sk-...
codex-dock auth set
```

Saves the `OPENAI_API_KEY` environment variable value to `~/.config/codex-dock/apikey`.
Protected with `0600` permissions.

### `auth rotate` — Rotate Tokens

```bash
codex-dock auth rotate
```

Invalidates all currently issued tokens.

---

## `codex-dock network` — Network Management

Manages the `dock-net` Docker network.

### `network create` — Create Network

```bash
codex-dock network create [--no-internet]
```

| Option | Description |
|---|---|
| `--no-internet` | Disable IP Masquerade to block internet access |

> Created automatically when `codex-dock run` is executed.

### `network rm` — Delete Network

```bash
codex-dock network rm
```

> Stop any running containers first.

### `network status` — Check Network Status

```bash
codex-dock network status
```

**Example output:**
```
dock-net status:
  ID:            a1b2c3d4e5f6789012345678
  Driver:        bridge
  Subnet:        192.168.200.0/24
  ICC:           disabled
  IP Masquerade: enabled
```

---

## `codex-dock build` — Build Image

Builds the Docker image for the sandbox.

```bash
codex-dock build [OPTIONS]
```

| Option | Short | Default | Description |
|---|---|---|---|
| `--tag` | `-t` | `codex-dock:latest` | Image tag |
| `--dockerfile` | `-f` | (auto-detect) | Path to Dockerfile |

### Dockerfile Search Order

If `-f` is omitted, Dockerfile is auto-detected in the following order:

1. `Dockerfile` in current directory
2. `docker/Dockerfile` in current directory
3. `~/.config/codex-dock/Dockerfile` (writes built-in default if not found)

> **Note**: When falling back to `~/.config/codex-dock/`, `entrypoint.sh` is also written to the same directory.
> Existing user-modified files are not overwritten.

**Examples:**

```bash
# Build with defaults (auto-detect Dockerfile)
codex-dock build

# Build with custom tag
codex-dock build --tag my-codex:v1

# Specify Dockerfile explicitly
codex-dock build -f /path/to/Dockerfile

# Build custom image and use with run
codex-dock build -f ./custom/Dockerfile --tag my-codex:v2
codex-dock run --image my-codex:v2
```

---

## `codex-dock ui` — TUI Dashboard

Launches a terminal UI for real-time monitoring and management of all workers.

```bash
codex-dock ui
```

### Screen Layout

```
┌──────────────────────────────────────────────────────────────┐
│ codex-dock  [running: 2 / total: 4]                          │
├──────────────────────────────────────────────────────────────┤
│  NAME              BRANCH      STATUS      UPTIME  TASK      │
│  codex-brave-atl   feature-1   running     5m23s   Write tests│
│▶ codex-calm-bea    main        running     2m10s   (interactive)│
│  codex-old-comet   feature-2   exited      -       done      │
├──────────────────────────────────────────────────────────────┤
│ [↑↓] select  [Enter] logs  [S] stop  [D] rm  [A] stop-all  [Q] quit │
└──────────────────────────────────────────────────────────────┘
```

### Key Bindings

| Key | Action | Status |
|---|---|---|
| `↑` / `↓` | Select container | ✅ |
| `Enter` | Show log view | ⚠️ stub text |
| `S` | Stop selected container | ✅ |
| `D` | Remove selected container | ✅ |
| `A` | Stop all containers | ✅ |
| `R` | Start container | ❌ not implemented |
| `Q` | Quit UI | ✅ |

> **Refresh interval**: Container list updates automatically every 2 seconds.
