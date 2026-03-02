# SKILL.md — codex-dock Reusable Task Patterns

> Domain-specific instructions and reusable patterns for AI agents working on `codex-dock`.
> Sources: `doc/architecture.md`, `doc/auth-proxy.md`, `doc/commands.md`, `.golangci.yml`, `.github/workflows/ci.yml`, `README.md`

---

## 1. Adding a New CLI Command

**Pattern**: All CLI commands use [Cobra](https://github.com/spf13/cobra).

```
cmd/<name>.go          ← Cobra command definition + flag binding
internal/<pkg>/        ← Business logic (keep cmd/ thin)
```

**Steps**:
1. Create `cmd/<name>.go` with a `var <name>Cmd = &cobra.Command{...}`.
2. Register it in `cmd/root.go` (or whichever file calls `rootCmd.AddCommand`).
3. Bind flags with `viper.BindPFlag` so they respect config file and env var precedence.
4. Implement business logic in `internal/<pkg>/`.
5. Add a test in `internal/<pkg>/<name>_test.go`.
6. Run `go vet ./...` and `golangci-lint run` before committing.

---

## 2. Adding a New Internal Package

**Pattern**: All business logic lives under `internal/`.

```
internal/<pkg>/
    <pkg>.go          ← Core implementation
    <pkg>_test.go     ← Unit tests
```

- Export only what `cmd/` needs.
- Do **not** import `cmd/` from `internal/` (would create a cycle).
- Error handling: return `error` values — never silently swallow errors.

---

## 3. Error Handling (errcheck Compliance)

The `errcheck` linter is enabled with `check-type-assertions: true`.

**Do**:
```go
if err := someFunc(); err != nil {
    return fmt.Errorf("context: %w", err)
}

// If truly ignorable, assign explicitly:
_ = optionalClose()
```

**Don't**:
```go
someFunc()          // ❌ errcheck will fail
f.Close()           // ❌ errcheck will fail
```

In `_test.go` files, errcheck is relaxed — you may write `someFunc()` directly in tests.

---

## 4. Auth Proxy Token Lifecycle

> Source: `doc/architecture.md`, `internal/authproxy/proxy.go`

The Auth Proxy is the security boundary. When writing code that starts or stops containers:

**Startup sequence** (already implemented in `cmd/run.go`):
1. Start Auth Proxy on a random host port (`127.0.0.1:<PORT>`).
2. Issue a short-lived token: `proxy.IssueToken(containerName, ttl)` → `cdx-xxxx`.
3. Pass `CODEX_TOKEN` and `CODEX_AUTH_PROXY_URL` as container env vars.

**Shutdown sequence** (F-AUTH-04 — **not yet implemented**):
```go
// TODO: call this when container stops
proxy.RevokeToken(token)
```
When implementing F-AUTH-04, call `RevokeToken` from `sandbox.Manager.Stop()` or the stop flow in `cmd/run.go`.

**OAuth vs API key**: The Auth Proxy auto-detects the mode by checking if `~/.codex/auth.json` contains a `refresh_token` field. The `/token` endpoint returns either `{"api_key": "..."}` or `{"oauth_access_token": "..."}`.

---

## 5. Docker Container Creation Pattern

> Source: `internal/sandbox/manager.go`, `internal/sandbox/types.go`

`RunOptions` (defined in `internal/sandbox/types.go`) is the canonical input struct. Add new container parameters there, then read them in `manager.go`.

Security settings that **must** be preserved on every container:
```go
HostConfig{
    CapDrop:        []string{"ALL"},
    SecurityOpt:    []string{"no-new-privileges"},
    PidsLimit:      ptr(int64(512)),
    NetworkMode:    "dock-net",
    // ReadOnly applied via Mounts[0].ReadOnly
}
```

Non-root user is enforced in `docker/Dockerfile` via `USER codex` (uid:1000).

---

## 6. Writing / Running Tests

**Test packages covered by CI**:
- `./internal/sandbox/...`
- `./internal/authproxy/...`
- `./internal/worktree/...`
- `./internal/config/...`

**Run all tests**:
```bash
go test -race -coverprofile=coverage.out -covermode=atomic \
  ./internal/sandbox/... \
  ./internal/authproxy/... \
  ./internal/worktree/... \
  ./internal/config/...
go tool cover -func=coverage.out
```

**Test file naming**: `<file>_test.go` in the same package. Use `_test` package suffix for black-box tests.

**Race conditions**: Always run with `-race` locally before pushing; CI enforces it.

---

## 7. Adding Configuration Options

Configuration uses [Viper](https://github.com/spf13/viper) with this precedence:
```
CLI flags > CODEX_DOCK_* env vars > config.toml > built-in defaults
```

**Steps**:
1. Add the field to the config struct (in `internal/config/` if it exists, otherwise in Viper initialization).
2. Bind the Cobra flag: `viper.BindPFlag("field_name", cmd.Flags().Lookup("flag-name"))`.
3. Set an env var override: `viper.BindEnv("field_name", "CODEX_DOCK_FIELD_NAME")`.
4. Document in `doc/configuration.md` and `configs/config.toml.example`.

---

## 8. Fixing Network Issues (F-NET-04)

> Source: `doc/architecture.md`, `doc/network.md`

**Problem**: Auth Proxy listens on `127.0.0.1` (host loopback), which is unreachable from containers on `dock-net`.

**Fix pattern**:
1. Determine the docker bridge gateway IP for `dock-net` (e.g., `192.168.200.1`).
2. Bind the Auth Proxy listener to that IP instead of `127.0.0.1`.
3. Set `CODEX_AUTH_PROXY_URL=http://192.168.200.1:<PORT>` in the container env.
4. Add an iptables rule to allow only Auth Proxy traffic from dock-net containers.

This fix also enables full NF-SEC-01 compliance (TLS or UNIX socket).

---

## 9. TUI (Terminal UI) Patterns

> Source: `internal/ui/ui.go`, `doc/commands.md`

The TUI uses `github.com/rivo/tview` (backed by `tcell`). Refresh interval: 2 seconds.

**Keybind registration pattern** (Cobra-style for TUI):
```go
switch event.Key() {
case tcell.KeyRune:
    switch event.Rune() {
    case 'S': // stop selected container
    case 'D': // delete selected container
    case 'A': // stop all containers
    case 'R': // TODO: start container (F-UI-02 — unimplemented)
    case 'Q': // quit
    }
}
```

**Log view** (F-UI-03): The log panel currently shows stub text. Fix by calling `mgr.Logs(containerName)` and feeding output to the tview `TextView`.

---

## 10. Pre-commit Checklist

Before every commit:

```bash
# 1. Format
gofmt -w ./...

# 2. Vet
go vet ./...

# 3. Lint
golangci-lint run --timeout=5m

# 4. Test
go test -race ./internal/sandbox/... ./internal/authproxy/... \
        ./internal/worktree/... ./internal/config/...

# 5. Module tidy (must produce no diff)
go mod tidy
git diff --exit-code go.mod go.sum
```

All five steps correspond to CI jobs (`lint`, `vet`, `test`, `build`) and must pass before pushing.

---

## 11. packages.dock File Format

> Source: `doc/commands.md`, `internal/sandbox/packages.go`

```
# Comment lines start with #
apt:libssl-dev        # apt package
pip:requests          # pip package
npm:typescript        # npm package
libssl-dev            # no prefix → treated as apt (F-PKG-05: auto-detection not implemented)
```

When adding auto-detection for F-PKG-05, implement heuristics in `internal/sandbox/packages.go`.

---

## 12. Cross-Platform Build Targets

> Source: `.github/workflows/ci.yml`

| Target | Command |
|---|---|
| Linux amd64 (native CI) | `go build -o codex-dock .` |
| macOS arm64 | `GOOS=darwin GOARCH=arm64 go build -o codex-dock-darwin-arm64 .` |
| macOS amd64 | `GOOS=darwin GOARCH=amd64 go build -o codex-dock-darwin-amd64 .` |
| Linux arm64 | `GOOS=linux GOARCH=arm64 go build -o codex-dock-linux-arm64 .` |

The binary has no runtime dependencies beyond a Docker socket.
