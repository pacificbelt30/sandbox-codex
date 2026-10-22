# `oaiso ui` — TUI Dashboard

> [日本語](../../commands/ui.md) | **English**
>
> [← Command Reference](../commands.md)

```bash
oaiso ui
```

Launches a terminal UI for real-time monitoring and management of all workers.

---

## Key Bindings

| Key | Action on the worker list |
|---|---|
| `↑` / `↓` | Select worker |
| `Enter` | Fetch the selected worker's last 200 log lines |
| `S` | Stop selected worker with a 10-second grace period |
| `D` | Force-remove selected worker |
| `A` | Stop all running workers in the list |
| `R` | Restart a stopped worker |
| `E` | Fetch selected worker's proxy denial history |
| `P` | Open proxy settings |
| `Q` | Quit; from log/denial view, return to the list |

- Letter keys accept either case.
- Stop, remove and stop-all execute without confirmation dialogs; save required artifacts first.
- Logs and denials are fetched on opening, not followed. For live following, use `logs --follow` or `policy events --worker WORKER --follow`.
- Authentication uses the same global settings as the CLI; [configure the profile and admin secret](../guides/authentication.md) first.

> **Refresh interval**: Container list updates every 2 seconds.

## Proxy settings

```bash
oaiso proxy configure
```

- Opens the editor directly, including when the node is stopped.
- Edits global settings, or the file selected by `oaiso --config PATH proxy configure`.
- Fields: allow/deny domains, CIDRs and ports; empty-grant default denial; upstream proxy URL; host CA path.

| Button or key | Action |
|---|---|
| Validate | Validate input syntax |
| Test destination | Check HOST:PORT by name/port only; no DNS or live connection |
| Save | Save settings; reject concurrent edits |
| Reload | Reload saved settings |
| Back / Esc | Return to the list, or exit the standalone editor |

- Does not edit API keys, real tokens or admin-secret values.
- Saving does not update live proxies; follow [network setup](../guides/networking.md) to recreate them.

---

## Related Documentation

- [Worker Management](worker.md)
- [`oaiso run`](run.md)
