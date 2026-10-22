# `oaiso update` — Update oaiso itself

> [日本語](../../commands/update.md) | **English**
>
> [← Command Reference](../commands.md)

Updates `oaiso` installations managed by `go install`.

```bash
oaiso update
```

---

## Behavior

This runs the following command and creates or maintains `oaiso` in the same Go bin directory:

```bash
go install github.com/pacificbelt30/codex-dock@latest
```

- Fails with an error if `go` is not found on `PATH`.
- The `codex-dock` binary and `oaiso` / `cxd` aliases are placed in `$(go env GOBIN)`, or `$(go env GOPATH)/bin` when GOBIN is unset.
- An unrelated file named `oaiso` in that directory is left untouched and causes an error.

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
