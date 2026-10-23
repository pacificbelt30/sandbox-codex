# `oaiso update` — Update oaiso itself

> [日本語](../../commands/update.md) | **English**
>
> [← Command Reference](../commands.md)

- Release installations: [rerun the release installer](../../setup.md#更新する) to update the CLI, node and helpers together.
- This command updates only CLI installations managed by `go install`.

```bash
oaiso update
```

---

## Behavior

This installs the canonical `oaiso` entry point in Go's bin directory:

```bash
go install github.com/pacificbelt30/codex-dock/cmd/oaiso@latest
```

- Fails with an error if `go` is not found on `PATH`.
- For source-installed CLIs. Release installations use the release installer without adding Go.
- `oaiso` is placed in `$(go env GOBIN)`, or `$(go env GOPATH)/bin` when GOBIN is unset.
- No legacy commands or aliases are generated. For existing old files, see [migration](../../migration.md#oaisoへの名称変更) (Japanese).

## Update according to installation method

| Installation | Update procedure |
|---|---|
| Automatic installation or node bundle | [Setup update](../../setup.md#更新する) (Japanese); keep CLI, node and helpers on matching versions |
| Manual host installation | [Runtime and binary update](../../kata.md#固定資産の更新) (Japanese) |
| CLI only in Go's bin directory | `oaiso update`; node and images require separate updates |

- Check the active path with `command -v oaiso`.
- This does not update `/usr/local/bin/oaiso`; it installs a separate CLI in Go's bin directory.
- `make install` alone does not update the node service; use the appropriate host procedure above.

---

## Related Documentation

- [Installation](../installation.md) — First installation and supported hosts

## Host services and images

- This command does not update the node, guard, IO logger, pinned assets or proxy images.
- See [host setup](../../kata.md) and [operations](../../operations.md) (Japanese).
