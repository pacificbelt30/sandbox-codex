# Worker Management — ps / stop / rm / logs

> [日本語](../../commands/worker.md) | **English**
>
> [← Command Reference](../commands.md)

---

## `oaiso ps`

```bash
oaiso ps [--all]
```

Lists running (and optionally stopped) containers.

```
NAME                   STATUS    UPTIME    BRANCH         TASK
codex-brave-atlas      running   5m23s     feature-auth   Write unit tests
codex-calm-beacon      running   2m10s     main           (interactive)
```

---

## `oaiso start`

```bash
oaiso start NAME_OR_ID
```

- Restart a retained worker with the same ID and workspace.
- Kata checks run again and the saved placeholder is restored before user code is released.
- If a pinned proxy IP changed, create a new worker instead.
- `oaiso vm start` remains available for Kata VM inventory recovery.

## `oaiso stop`

```bash
oaiso stop [NAME|ID...] [--all] [--timeout 10]
```

> Stopping a container also immediately revokes its Auth Proxy token.
> See [Token Lifecycle](../auth-proxy/tokens.md).

---

## `oaiso rm`

```bash
oaiso rm [NAME|ID...] [--force]
```

---

## `oaiso logs`

```bash
oaiso logs NAME|ID [--tail 100] [--follow]
```

---

## Related Documentation

- [TUI Dashboard](ui.md)
- [`oaiso run`](run.md)
- [Quick Start](../getting-started.md)

See [VM management](../../vm.md) (Japanese) for host process and allocation reconciliation.
