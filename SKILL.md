# codex-dock development patterns

Read [CLAUDE.md](CLAUDE.md) for conventions and [architecture](doc/architecture.md) for the current containerd/Kata/Dragonball design.
These patterns describe this repository. They do not replace the host setup or security checks.

## Add a command or setting

1. Put Cobra wiring in `cmd/` and business logic in `internal/`.
2. Register the command and preserve explicitly supplied flags over config defaults.
3. Wire environment overrides explicitly; do not assume a `CODEX_DOCK_*` name works automatically.
4. Update `configs/config.toml.example` and both configuration references.
5. Add focused tests for new runtime behavior or contracts.

Do not import `cmd/` from internal packages. Avoid new global state.

## Change worker lifecycle

Use `sandbox.RunOptions` and the existing Manager. The engine talks to the dedicated node, not a Docker daemon.
New and restarted workers must pass pinned asset, host process/cgroup and guest restriction checks before the guard releases user code.
Keep token activation after successful verification.

Stop revokes the token and preserves restart state. Restart restores the placeholder for the same worker ID.
Remove persists intent first and retains cleanup retries for network, token, worktree and cgroup failures.
Use existing ownership checks for recovery; do not kill arbitrary PIDs or delete unknown resources.

## Change AI proxy behavior

Keep credential loading and refresh in `internal/authproxy`, separate from sandbox management.
Use the `Service` boundary for management calls.

- `worker_auth.go`: authenticate issued tokens before upstream access.
- `reverse.go`, `websocket.go`: shared forwarding with a provider-specific injector.
- `openai.go`, `anthropic.go`: provider credentials and refresh.
- `tokens.go`, `admin.go`, `restore.go`: leases and management API.
- `forward.go`, `transport.go`: credential-free general egress.
- `internal/networkpolicy`: destination rules independent of authentication and engine details.

Preserve HTTP/SSE/WS cancellation on revoke, expiry, replacement, restore and stop.
Credential-file changes must keep caller UID/GID, private HOME, read-only mounts and CapDrop ALL.
Use local upstream fixtures for credential injection and rejection tests.

## Change network behavior

Use `internal/network` for worker network lifecycle and `internal/engine/network*.go` for native host resources.
Worker names resolve through static hosts, not Docker embedded DNS.
Admin remains on the primary egress interface, separate from worker-facing data.
Do not reintroduce a shared worker network or bypass destination policy through a generic upstream proxy.

General allow/deny rules belong in `internal/networkpolicy` and must apply consistently to HTTP, redirects and CONNECT.
Private IP blocks, full DNS-answer checks and fixed-IP dialing must remain effective.
See [policy extension boundaries](doc/proxy-policy.md#コードの境界).

## Change images and templates

Build through `internal/engine.Build` and standalone BuildKit. Import into the same containerd namespace.
Keep the auth entrypoint protocol compatible; expose the matching script through `build --print-entrypoint`.
User templates under `~/.config/codex-dock/templates/<name>/Dockerfile` override built-ins.
Preserve tag resolution, validation and on-disk Dockerfile timestamp checks.
Package auto-detection already exists; use explicit `apt:`, `pip:` and `npm:` prefixes in examples.

## Check a change

Use gofmt for Go files. Return contextual errors, close bodies and use request contexts.
Run focused tests, then the required checks for the affected behavior:

```bash
make build
make test
make vet
make lint
```

Runtime changes also need `make kata-e2e` on a prepared Linux/KVM host.
Documentation changes need `mkdocs build --strict`.
See [testing](doc/testing.md) for live AI checks and limitations.
Do not put real credentials, tokens or private acceptance logs into the repository.
