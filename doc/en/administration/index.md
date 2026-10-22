# Administration

[日本語](../../operations.md) | **English**

- Prerequisite: an [installed execution host](../installation.md).
- Operational procedures below are currently available in Japanese; the performance guide is also in English.

## Daily operations

| Task | Guide |
|---|---|
| List workers, follow logs, stop, restart and remove | [Operations](../../operations.md) (Japanese), [worker commands](../commands/worker.md) |
| Inspect VM ownership and remaining resources | [VM management](../../vm.md) (Japanese) |
| Inspect or manage workers interactively | [TUI](../commands/ui.md) |

## Host maintenance

| Task | Guide |
|---|---|
| Update an automatically installed node | [Host installation and updates](../../setup.md) (Japanese) |
| Install or maintain a host manually | [Kata host setup](../../kata.md) (Japanese) |
| Migrate a legacy installation | [Migration](../../migration.md) (Japanese) |
| Diagnose startup, credentials or networking | [Troubleshooting](../../troubleshooting.md) (Japanese) |
| Compare performance and reproduce benchmarks | [Performance](../performance.md) |

- Save work before maintenance; follow the linked procedure to drain and reclaim workers.
- Normal worker commands run as the operator; host installation uses administrator privileges.
