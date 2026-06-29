# CLAUDE.md — codex-dock

> AI agent reference for the `codex-dock` repository.
> Sources: `README.md`, `go.mod`, `.golangci.yml`, `.github/workflows/ci.yml`, `doc/architecture.md`, `doc/commands.md`, `doc/configuration.md`, `doc/getting-started.md`

---

## Project Overview

**codex-dock** is an AI Sandbox Container Manager written in Go.
It runs AI coding agents — [Codex CLI](https://github.com/openai/codex) and
[Claude Code](https://github.com/anthropics/claude-code) — inside isolated Docker
containers. The agent is selected per worker with `--agent codex|claude`; omitting
`--agent` drops into an auth-configured interactive shell where both CLIs are
available. Key capabilities:

- **Auth Proxy** — OpenAI **and** Anthropic API keys / OAuth tokens never touch containers; short-lived tokens are injected instead.
- **Two-proxy router + per-worker networks** — egress is split by role into two containers: `codex-auth-proxy` (role `auth`: credential-injecting API reverse routes + token + admin; **does NOT forward general traffic**) and `codex-http-proxy` (role `egress`: forward proxy only for git/npm/pip, **no credentials**, private/LAN-blocked). Both are multi-homed onto each worker's `Internal` network. Workers reach only the proxies; worker↔worker, worker→host, and worker→internet are blocked by Docker itself (no iptables, no sudo).
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
│   ├── authproxy/            Auth Proxy (HTTP server + token lifecycle) — PROXY side
│   │   ├── proxy.go          HTTP server, token issuance, OpenAI + Anthropic reverse proxy
│   │   ├── auth.go           OpenAI + Anthropic API key / OAuth credential loading
│   │   ├── remote.go         RemoteProxy client (talks to a proxy container)
│   │   └── service.go        Service interface (in-process Proxy or RemoteProxy)
│   ├── sandbox/              Docker container lifecycle — SANDBOX side
│   │   ├── manager.go        Create / start / stop / rm containers, buildEnv (agent-aware)
│   │   ├── types.go          RunOptions, Agent (codex/claude/shell), ApprovalMode
│   │   └── packages.go       packages.dock file parsing
│   ├── network/              Docker network lifecycle (egress + per-worker Internal nets)
│   │   └── manager.go        EnsureEgressNetwork / EnsureWorkerNetwork / ConnectProxy / DisconnectProxy / RemoveWorkerNetwork
│   ├── template/             Sandbox image template management
│   │   ├── template.go       Template registry, resolution (Get/List/MatchTag), tag generation
│   │   └── validate.go       Static Dockerfile validation (required tools check)
│   ├── worktree/             git worktree management
│   │   └── worktree.go       Create / delete worktrees
│   └── ui/                   Terminal UI (tcell / tview)
│       └── ui.go             TUI dashboard (Bubble Tea-style keybinds)
│
├── docker/                   Container assets, split by side
│   ├── defaults.go           //go:embed of the Dockerfiles + entrypoint + templates
│   ├── sandbox/              SANDBOX image (= "plain" template)
│   │   ├── Dockerfile        Node.js 22 + Codex CLI + Claude Code, non-root uid:1001
│   │   └── entrypoint.sh     Startup: fetch auth from proxy, launch codex/claude/shell
│   ├── templates/            Image templates (one subdir per template)
│   │   └── pwn/Dockerfile    CTF/RE template: pwntools, ptrlib, ropper, pwndbg, radare2, etc.
│   └── proxy/               PROXY image
│       └── Dockerfile        Distroless Go build of the auth proxy (`proxy serve`)
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
  ./internal/config/... \
  ./internal/template/...

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

# Build using a template (plain = default, pwn = CTF/RE tools)
codex-dock build --template plain
codex-dock build --template pwn        # → codex-dock:pwn

# List available templates
codex-dock build --template list

# Or build directly (sandbox image: Codex CLI + Claude Code)
docker build -t codex-dock:latest -f docker/sandbox/Dockerfile docker/sandbox/

# Auth proxy image (build context is the repo root — it compiles the Go binary)
docker build -t codex-dock-proxy:latest -f docker/proxy/Dockerfile .
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
API Key ──▶ Auth Proxy (127.0.0.1) ──▶ placeholder token ──▶ container
                  │                     (cdx-xxxx, TTL-scoped)
                  │
                  └─ injects real Authorization header on every outbound request
```

- Containers receive only a **placeholder token** (`cdx-xxxx`) — never the real API key or OAuth access_token.
- The proxy overwrites `Authorization` (and `ChatGPT-Account-Id` in OpenAI OAuth mode) on every proxied request with the real host credentials.
- OAuth refresh responses also have `access_token` replaced with the placeholder before returning to containers.
- **Anthropic / Claude Code** follows the same model: containers set `ANTHROPIC_BASE_URL=http://<proxy>/anthropic` and a placeholder `ANTHROPIC_API_KEY`. The proxy injects the real credential on `/anthropic/*` requests — `x-api-key` (API-key mode) or `Authorization: Bearer …` + the `anthropic-beta` OAuth header (Claude subscription mode). In OAuth mode the proxy refreshes the access token itself; the refresh token stays on the host (`injectAnthropicCredentials` / `refreshAnthropicOAuthIfNeeded`).
- Containers run as non-root (`uid:1001`, `USER codex`).
- `--cap-drop ALL` + `--security-opt no-new-privileges` + `--pids-limit 512`.
- **Network isolation is 100% Docker-native (no iptables/sudo)**: each worker is on its own `Internal` bridge (`dock-net-w-<name>`). Different L2 segments block worker↔worker; `Internal: true` blocks worker→host/internet. Both proxies (`codex-auth-proxy`, `codex-http-proxy`) are multi-homed (egress net `dock-net-proxy` + each worker net) and are the only egress.
- **Credential isolation by role**: `codex-auth-proxy` holds the real credentials but only ever sends them to its three hardcoded upstreams (api.openai.com / chatgpt.com / api.anthropic.com) and refuses to forward arbitrary traffic. `codex-http-proxy` does the arbitrary forwarding but holds no credentials and blocks private/LAN destinations (`--block-private`: RFC1918, 127/8, 169.254/16 incl. cloud metadata, ULA, CGNAT). `/admin/*` is on a separate listener bound to the egress IP so workers can't reach token issuance.
- Container `auth.json` contains a placeholder `access_token`; `refresh_token` is empty.
- `CODEX_TOKEN` (`cdx-xxxx…`) expires on TTL; revoked on container stop.

---

## Known Implementation Gaps

> Source: `README.md` implementation status table

| ID | Issue | Notes |
|---|---|---|
| NF-SEC-01 | Auth Proxy uses plain HTTP | TLS or UNIX socket required per SRS |
| F-UI-02 | TUI `[R]` start key unimplemented | Key handler missing |
| F-UI-03 | TUI log view shows stub text | `mgr.Logs()` not called |
| `--agents-md` | `CODEX_AGENTS_MD` env var not set in container | `entrypoint.sh` handler exists but env var not injected |
| `mountMode` | `ReadOnly` applied via `Mounts[0].ReadOnly`, `mountMode` var is dead code | |

**Resolved gaps** (no longer applicable):

| ID | Resolution |
|---|---|
| F-AUTH-04 | `proxy.RevokeToken()` is called from `sandbox.Manager.Stop()` and `runSingle()` |
| access_token leak | Containers now receive only a placeholder; proxy injects real credentials on every outbound request (`injectCredentials`) |
| F-NET-01/02/04 | Replaced the iptables firewall with Docker-native isolation: per-worker `Internal` networks + multi-homed proxy router. Workers reach the proxy via Docker DNS (`codex-auth-proxy`), no `host.docker.internal`/`--add-host`, no iptables/sudo. See `internal/network/manager.go` (`EnsureWorkerNetwork`/`ConnectProxy`) and `sandbox.Manager.Run`. |
| iptables firewall | `internal/network/firewall.go` and the `codex-dock firewall` command group were removed; `--sudo/--no-firewall/--allow-host/--block-host` flags are gone. |

---

## CI Pipeline

Source: `.github/workflows/ci.yml`

| Job | What it does |
|---|---|
| `lint` | `golangci-lint run --timeout=5m` |
| `build` | `go build`, cross-compile darwin/arm64, darwin/amd64, linux/arm64 |
| `test` | `go test -race -coverprofile` on `cmd`, `internal/sandbox`, `authproxy`, `network`, `worktree`, `config`, `template` |
| `vet` | `go vet ./...` + `go mod tidy` idempotency check |
| `docker` | `docker buildx build` of `docker/sandbox/Dockerfile` and `docker/proxy/Dockerfile` (no push) |

Triggers: all pushes and pull requests on all branches.

---

## Configuration

User config file: `~/.config/codex-dock/config.toml`

```toml
default_image     = "codex-dock:latest"   # --image
default_token_ttl = 3600                  # --token-ttl (seconds)
network_name      = "dock-net-proxy"     # egress network the proxy attaches to
verbose           = false                 # --verbose
debug             = false                 # --debug
```

Environment variable overrides follow the pattern `CODEX_DOCK_<SETTING_NAME>` (e.g., `CODEX_DOCK_VERBOSE`).

Auth files:

| File | Path | Provider | Description |
|---|---|---|---|
| OpenAI API key | `~/.config/codex-dock/apikey` | Codex | Written by `codex-dock auth set` (perm 0600) |
| OpenAI OAuth | `~/.codex/auth.json` | Codex | Written by Codex CLI `codex login` |
| Anthropic API key | `~/.config/codex-dock/anthropic-apikey` | Claude | Or the `ANTHROPIC_API_KEY` env var |
| Anthropic OAuth | `~/.claude/.credentials.json` | Claude | Written by `claude` subscription login (`claudeAiOauth`) |

The proxy loads each provider independently, so one proxy can serve both agents.

---

## Agents & Container Environment

`--agent` selects the agent the sandbox launches (`DOCK_AGENT` inside the container):

| `--agent` | `DOCK_AGENT` | Behaviour | Proxy env injected by `manager.buildEnv` |
|---|---|---|---|
| _(omitted)_ | `""` | Auth-configured interactive shell (both CLIs on PATH) | Codex **and** Anthropic vars (when available) + `HTTP(S)_PROXY`/`NO_PROXY` |
| `codex` | `codex` | Launch Codex CLI | `CODEX_AUTH_PROXY_URL`, `CODEX_TOKEN`, `OPENAI_BASE_URL`, OAuth refresh override, `HTTP(S)_PROXY` |
| `claude` | `claude` | Launch Claude Code | `ANTHROPIC_BASE_URL` (`…/anthropic`), `ANTHROPIC_API_KEY` (placeholder), `HTTP(S)_PROXY` |

`HTTP_PROXY`/`HTTPS_PROXY` point at the **egress** proxy (`http://codex-http-proxy:18082`) so general egress (git/npm/pip) routes through the forward proxy; `OPENAI_/ANTHROPIC_BASE_URL` point at the **auth** proxy (`http://codex-auth-proxy:18080/…`); `NO_PROXY=codex-auth-proxy,localhost,127.0.0.1` keeps token/API traffic direct (origin-form → reverse routes). `--no-internet` omits the `HTTP(S)_PROXY` vars (API reverse routes only).

`--shell` still bypasses `entrypoint.sh` entirely for a raw, **un-authenticated** debug shell.
`run --agent claude` fails fast if the proxy reports no Anthropic credentials
(`RemoteProxy.IsAnthropicMode()` via `/admin/mode`).

---

## Notes for AI Agents

1. **Always run `gofmt`** before committing. The CI `gofmt` check is strict.
2. **Check all errors** — `errcheck` will fail if you discard errors without assigning to `_` explicitly.
3. **Test coverage targets**: `internal/sandbox`, `internal/authproxy`, `internal/worktree`, `internal/config`, `internal/template`.
4. **No new global state** — config flows through Viper/Cobra flag bindings.
5. **`internal/` is the right place** for new business logic; `cmd/` is for CLI wiring only.
6. **Do not pass credentials to containers** — containers receive only placeholder `cdx-xxxx` tokens. The proxy injects real credentials on every outbound request (`injectCredentials` for OpenAI, `injectAnthropicCredentials` for Anthropic).
7. **`reverseProxy` takes an injector** — `reverseProxy(w, r, prefix, base, inject)`. OpenAI routes pass `p.injectCredentials`; the `/anthropic` route passes `p.injectAnthropicCredentials`. Any new proxy endpoint must pass an appropriate injector. The WebSocket path (`handleWebSocketProxy`) is OpenAI-only; Anthropic uses HTTP/SSE.
8. **Proxy vs Sandbox separation** — proxy logic lives in `internal/authproxy` + `docker/proxy`; sandbox logic in `internal/sandbox` + `docker/sandbox`. They communicate only over the proxy's HTTP API (`Service` interface). Keep credential handling on the proxy side.
8. **Network isolation is Docker-native (router model)**: workers reach the proxy via Docker embedded DNS (`codex-auth-proxy`) on their per-worker `Internal` network — no `host.docker.internal`/`--add-host`, no iptables. `sandbox.Manager.Run` calls `network.EnsureWorkerNetwork` + `ConnectProxy` before creating the container; `Remove`/`RemoveByName` call `cleanupWorkerNetwork`. The proxy is the only egress. When adding network behavior, keep it inside Docker primitives (Internal nets, ICC) rather than reintroducing iptables.
9. **Documentation is in Japanese** (`doc/`). New docs may be written in English or Japanese consistently with existing files.
10. **`go mod tidy` must leave `go.mod`/`go.sum` clean** — CI checks this with `git diff --exit-code`.
11. **Image templates** — `internal/template` manages sandbox image templates. `plain` maps to the existing `docker/sandbox/Dockerfile`; derived templates (e.g. `pwn`) live under `docker/templates/<name>/Dockerfile` and `FROM codex-dock:latest`. Templates are embedded via `embed.FS` in `docker/defaults.go`. Use `template.Get(name)` / `template.List()` to access them. `template.Validate()` checks that base templates include required tools (codex, claude-code, git, curl, non-root user, entrypoint).
