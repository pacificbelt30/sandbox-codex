# Dependency checks during age migration

[日本語](../../development/dependency-security.md) | **English**

## Results (2026-10-24)

| Item | Result |
|---|---|
| Tool/environment | govulncheck v1.9.0, Go 1.27.3, Linux amd64, all Go packages |
| Baseline | Separate checkout of pre-age commit `1168123` |
| Baseline symbol findings | 19 |
| age plus patched HTTP/2 dependencies | 15; zero findings absent from the baseline |
| Outstanding | 15 existing dependency findings; exploitability review and compatible updates remain separate work |

- **This dependency check is not clean.** The 15 findings remain unresolved.
- Counts reflect static analysis and the Go vulnerability database, not confirmed exploitability. This page is not a completed security audit.
- [govulncheck documentation](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck).

## Applied updates

| Target | Action |
|---|---|
| age | Pin v1.3.2; no age finding reported |
| `golang.org/x/net` | Upgrade age's resolved v0.57.0 to patched HTTP/2 release v0.60.0 |
| Related dependencies | Align x/crypto, x/sys, x/term, x/text and x/sync through Go module resolution |
| Go | Minimum 1.26.0 required by patched dependencies; development/CI use 1.27.3; binary users need no Go |

- [GO-2026-6610](https://pkg.go.dev/vuln/GO-2026-6610) appeared during intermediate resolution and was removed by the patch upgrade.
- Baseline findings [GO-2026-6603](https://pkg.go.dev/vuln/GO-2026-6603), [GO-2026-6611](https://pkg.go.dev/vuln/GO-2026-6611), [GO-2026-6612](https://pkg.go.dev/vuln/GO-2026-6612) and [GO-2026-6617](https://pkg.go.dev/vuln/GO-2026-6617) no longer appear.

## Existing dependency backlog

| Go module | Current version | Symbol findings | Follow-up |
|---|---|---:|---|
| containerd SDK | v2.1.5 | 8 | Distinguish used SDK paths from daemon-only features; validate compatible updates |
| Docker SDK/types | v28.5.2 | 2 | Review client/DTO usage separately from daemon implementation |
| gRPC | v1.78.0 | 2 | Review node/containerd reachability and compatibility |
| OpenTelemetry | v1.40.0 | 1 | Review enabled instrumentation paths and update |
| moby/sys/user | v0.4.0 | 1 | Check UID/GID resolution behavior and update |
| SELinux library | v1.12.0 | 1 | Review mount/label reachability and update |

- The Go containerd SDK version differs from pinned daemon version 2.4.2; SDK findings alone do not establish the daemon's exposure.
- Validate larger SDK upgrades through separate patches and real Kata acceptance rather than combining them with this storage-format migration.
- Raw local records remain under `.kata-verification/age-20261024/`; credentials/private runtime logs are not committed.
