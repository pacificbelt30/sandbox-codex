# Worker Management — ps / stop / rm / logs

> [日本語](../../commands/worker.md) | **English**
>
> [← Command Reference](../commands.md)

---

## `codex-dock ps`

```bash
codex-dock ps [--all]
```

Lists running (and optionally stopped) containers.

```
NAME                   STATUS    UPTIME    BRANCH         TASK
codex-brave-atlas      running   5m23s     feature-auth   Write unit tests
codex-calm-beacon      running   2m10s     main           (interactive)
```

---

## `codex-dock vm start`

```bash
codex-dock vm start NAME_OR_ID
```

Restart a retained worker with the same ID and workspace.
Kata checks run again and the saved placeholder is restored before user code is released.
If a pinned proxy IP changed, create a new worker instead.

## `codex-dock stop`

```bash
codex-dock stop [NAME|ID...] [--all] [--timeout 10]
```

> Stopping a container also immediately revokes its Auth Proxy token.
> See [Token Lifecycle](../auth-proxy/tokens.md).

---

## `codex-dock rm`

```bash
codex-dock rm [NAME|ID...] [--force]
```

---

## `codex-dock logs`

```bash
codex-dock logs NAME|ID [--tail 100] [--follow]
```

---

## Related Documentation

- [TUI Dashboard](ui.md)
- [`codex-dock run`](run.md)
- [Quick Start](../getting-started.md)

See [VM management](../../vm.md) (Japanese) for host process and allocation reconciliation.
