# CLAUDE.md — codex-dock

> AI agent reference for the `codex-dock` repository.
> Sources: `README.md`, `go.mod`, `.golangci.yml`, `.github/workflows/ci.yml`, `doc/architecture.md`, `doc/commands.md`, `doc/configuration.md`, `doc/getting-started.md`

---

## Project Overview

**codex-dock** is an AI Sandbox Container Manager written in Go.
It runs [Codex CLI](https://github.com/openai/codex) inside isolated Docker containers with:

- **Auth Proxy** — API keys / OAuth tokens never touch containers; short-lived tokens are injected instead.
- **dock-net** — Dedicated Docker bridge network with ICC disabled and host access blocked.
- **git worktree** — Parallel development branches, each in their own container.
- **dock-ui** — Terminal UI (TUI) for managing all workers.
- **Package management** — `apt`, `pip`, `npm` packages via `--pkg` or `packages.dock`.

Module: `github.com/pacificbelt30/codex-dock`

---

## Directory Structure

```
codex-dock/
├── main.go                   Entry point — delegates to cmd.Execute()
├── go.mod / go.sum           Go module definition
├── .golangci.yml             Lint configuration (golangci-lint)
├── .gitignore
│
├── cmd/                      Cobra CLI commands
│   ├── run.go                `codex-dock run` — container launch & worker management
│   ├── auth.go               `codex-dock auth` — auth set / show / rotate
│   ├── build.go              `codex-dock build` — sandbox image build
│   ├── ps.go                 `codex-dock ps` — list workers
│   ├── stop.go               `codex-dock stop`
│   ├── rm.go                 `codex-dock rm`
│   ├── logs.go               `codex-dock logs`
│   ├── ui.go                 `codex-dock ui` — TUI dashboard
│   └── network.go            `codex-dock network` — dock-net management
│
├── internal/
│   ├── authproxy/            Auth Proxy (HTTP server + token lifecycle)
│   │   ├── proxy.go          HTTP server, token issuance & revocation
│   │   └── auth.go           API key / OAuth credential loading
│   ├── sandbox/              Docker container lifecycle
│   │   ├── manager.go        Create / start / stop / rm containers
│   │   ├── types.go          RunOptions and related types
│   │   └── packages.go       packages.dock file parsing
│   ├── network/              dock-net Docker bridge management
│   │   └── manager.go        Create / delete / inspect network
│   ├── worktree/             git worktree management
│   │   └── worktree.go       Create / delete worktrees
│   └── ui/                   Terminal UI (tcell / tview)
│       └── ui.go             TUI dashboard (Bubble Tea-style keybinds)
│
├── docker/
│   ├── Dockerfile            Sandbox image (Node.js 22 + Codex CLI, non-root uid:1000)
│   └── entrypoint.sh         Container startup: fetches auth from proxy, launches Codex
│
├── configs/
│   └── config.toml.example   Example config for ~/.config/codex-dock/config.toml
│
└── doc/                      Japanese documentation
    ├── index.md
    ├── architecture.md
    ├── auth-proxy.md
    ├── commands.md
    ├── configuration.md
    ├── getting-started.md
    └── network.md
```

---

## Tech Stack

| Component | Library / Tool | Version |
|---|---|---|
| Language | Go | 1.24.7 (see `go.mod`) |
| CLI framework | `github.com/spf13/cobra` | 1.10.2 |
| Config management | `github.com/spf13/viper` | 1.21.0 |
| Docker API | `github.com/docker/docker` | 28.5.2 |
| TUI (terminal cells) | `github.com/gdamore/tcell/v2` | 2.13.8 |
| TUI (widgets) | `github.com/rivo/tview` | 0.42.0 |
| Linter | `golangci-lint` | latest (CI) |

---

## Build, Test, and Lint Commands

### Build

```bash
# Build binary for the current platform
go build -o codex-dock .

# Cross-compile (examples from CI)
GOOS=darwin GOARCH=arm64 go build -o codex-dock-darwin-arm64 .
GOOS=darwin GOARCH=amd64 go build -o codex-dock-darwin-amd64 .
GOOS=linux  GOARCH=arm64 go build -o codex-dock-linux-arm64 .
```

### Test

```bash
# Run all tested packages with race detection and coverage
go test \
  -race \
  -coverprofile=coverage.out \
  -covermode=atomic \
  ./internal/sandbox/... \
  ./internal/authproxy/... \
  ./internal/worktree/... \
  ./internal/config/...

# View coverage report
go tool cover -func=coverage.out
```

> Source: `.github/workflows/ci.yml` — `test` job

### Lint

```bash
# Run golangci-lint (matches CI)
golangci-lint run --timeout=5m
```

### Vet + Module Tidy

```bash
go vet ./...
go mod tidy
git diff --exit-code go.mod go.sum   # ensure go.mod stays tidy
```

### Docker Sandbox Image

```bash
# Build using the codex-dock CLI (wraps docker build)
codex-dock build

# Or build directly
docker build -t codex-dock:latest -f docker/Dockerfile docker/
```

---

## Coding Conventions

Source: `.golangci.yml`

- **Formatting**: `gofmt` with `simplify: true`. All code must be gofmt-formatted before committing.
- **Error handling**: All errors must be checked (`errcheck`). Type assertions are also checked. In test files (`_test.go`), errcheck is relaxed.
- **Linters enabled**: `errcheck`, `govet`, `ineffassign`, `staticcheck`, `unused`, `gofmt`, `misspell`, `gosimple`, `bodyclose`, `noctx`.
- **No unused variables or imports**: enforced by `unused` and `ineffassign`.
- **HTTP response bodies**: must be closed (`bodyclose`).
- **Context**: HTTP requests must use `context` (`noctx`).
- **Go idioms**: follow `gosimple` suggestions.

### General Style

- Single binary distribution — no external runtime dependencies.
- Packages live under `internal/`; public API surface is minimal.
- CLI commands in `cmd/` use Cobra and delegate logic to `internal/` packages.
- Configuration uses Viper with priority: CLI flags > `CODEX_DOCK_*` env vars > `config.toml` > built-in defaults.

---

## Security Principles

> Source: `doc/architecture.md`, `README.md`

**Core rule**: API keys and OAuth credentials never reach containers directly.

```
API Key ──▶ Auth Proxy (127.0.0.1) ──▶ short-lived token ──▶ container (TTL-scoped)
```

- Containers run as non-root (`uid:1000`, `USER codex`).
- `--cap-drop ALL` + `--security-opt no-new-privileges` + `--pids-limit 512`.
- `dock-net` bridge network with ICC (inter-container communication) disabled.
- `auth.json` is never bind-mounted into containers.
- `CODEX_TOKEN` (`cdx-xxxx…`) expires on TTL; revocation on container stop is **not yet wired** (F-AUTH-04).

---

## Known Implementation Gaps

> Source: `README.md` implementation status table

| ID | Issue | Notes |
|---|---|---|
| F-AUTH-04 | Token not revoked on container stop | `proxy.RevokeToken()` exists but is not called from `sandbox.Manager.Stop()` |
| F-NET-04 | Auth Proxy unreachable from dock-net | Proxy listens on `127.0.0.1` (loopback), not inside dock-net |
| NF-SEC-01 | Auth Proxy uses plain HTTP | TLS or UNIX socket required per SRS |
| F-UI-02 | TUI `[R]` start key unimplemented | Key handler missing |
| F-UI-03 | TUI log view shows stub text | `mgr.Logs()` not called |
| `--agents-md` | `CODEX_AGENTS_MD` env var not set in container | `entrypoint.sh` handler exists but env var not injected |
| `mountMode` | `ReadOnly` applied via `Mounts[0].ReadOnly`, `mountMode` var is dead code | |

---

## CI Pipeline

Source: `.github/workflows/ci.yml`

| Job | What it does |
|---|---|
| `lint` | `golangci-lint run --timeout=5m` |
| `build` | `go build`, cross-compile darwin/arm64, darwin/amd64, linux/arm64 |
| `test` | `go test -race -coverprofile` on `internal/sandbox`, `authproxy`, `worktree`, `config` |
| `vet` | `go vet ./...` + `go mod tidy` idempotency check |
| `docker` | `docker buildx build` of `docker/Dockerfile` (no push) |

Triggers: all pushes and pull requests on all branches.

---

## Configuration

User config file: `~/.config/codex-dock/config.toml`

```toml
default_image     = "codex-dock:latest"   # --image
default_token_ttl = 3600                  # --token-ttl (seconds)
network_name      = "dock-net"
verbose           = false                 # --verbose
debug             = false                 # --debug
```

Environment variable overrides follow the pattern `CODEX_DOCK_<SETTING_NAME>` (e.g., `CODEX_DOCK_VERBOSE`).

Auth files:

| File | Path | Description |
|---|---|---|
| API key | `~/.config/codex-dock/apikey` | Written by `codex-dock auth set` (perm 0600) |
| OAuth creds | `~/.codex/auth.json` | Written by Codex CLI `codex login` |

---

## Notes for AI Agents

1. **Always run `gofmt`** before committing. The CI `gofmt` check is strict.
2. **Check all errors** — `errcheck` will fail if you discard errors without assigning to `_` explicitly.
3. **Test coverage targets**: `internal/sandbox`, `internal/authproxy`, `internal/worktree`, `internal/config`.
4. **No new global state** — config flows through Viper/Cobra flag bindings.
5. **`internal/` is the right place** for new business logic; `cmd/` is for CLI wiring only.
6. **Do not pass credentials to containers** — maintain the Auth Proxy pattern for any new auth flows.
7. **When fixing F-AUTH-04**, call `proxy.RevokeToken()` inside `sandbox.Manager.Stop()` or in the `cmd/run.go` stop flow.
8. **When fixing F-NET-04**, the Auth Proxy must bind to a dock-net interface address, not `127.0.0.1`.
9. **Documentation is in Japanese** (`doc/`). New docs may be written in English or Japanese consistently with existing files.
10. **`go mod tidy` must leave `go.mod`/`go.sum` clean** — CI checks this with `git diff --exit-code`.
