# `codex-dock update` — Update codex-dock itself

> [日本語](../../commands/update.md) | **English**
>
> [← Command Reference](../commands.md)

Updates the `codex-dock` binary to the latest version.

```bash
codex-dock update
```

---

## Behavior

This is a thin wrapper that runs:

```bash
go install github.com/pacificbelt30/codex-dock@latest
```

- Fails with an error if `go` is not found on `PATH`.
- The built binary is placed in `$(go env GOPATH)/bin` (usually `~/go/bin`).

> **Note**: This command only works for installs done via `go install`.
> If you installed with `make install` (e.g. into `/usr/local/bin`) or a
> prebuilt release binary, the binary you actually run lives at a different
> path than the one `go install` writes to, so running this command will not
> update it. In that case, run `git pull` and re-run `make install` instead.

---

## Related Documentation

- [Quick Start](../getting-started.md) — Installation via `go install`

## Host services and images

This command does not update the node, guard, IO logger, pinned assets or proxy images.
See [host setup](../../kata.md) and [operations](../../operations.md) (Japanese).
