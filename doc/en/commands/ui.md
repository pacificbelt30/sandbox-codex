# `codex-dock ui` — TUI Dashboard

> [日本語](../../commands/ui.md) | **English**
>
> [← Command Reference](../commands.md)

```bash
codex-dock ui
```

Launches a terminal UI for real-time monitoring and management of all workers.

---

## Key Bindings

| Key | Action | Status |
|---|---|---|
| `↑` / `↓` | Select container | ✅ |
| `Enter` | Show log view | ⚠️ stub |
| `S` | Stop selected container | ✅ |
| `D` | Delete selected container | ✅ |
| `A` | Stop all containers | ✅ |
| `R` | Start container | ❌ not implemented |
| `Q` | Quit UI | ✅ |

> **Refresh interval**: Container list updates every 2 seconds.

---

## Related Documentation

- [Worker Management](worker.md)
- [`codex-dock run`](run.md)
