# Command Reference

> [日本語](../commands.md) | **English**

Full command reference for codex-dock. See the individual pages below for details on each command.

---

## Command Index

| Command | Description | Details |
|---|---|---|
| [`cxd codex`](commands/run.md#launch-codex-cli-directly) | Pass Codex CLI arguments through unchanged | Mount current directory and exec Codex |
| [`cxd run`](commands/run.md) | Start a sandbox container | Options, approval modes, parallel execution, package management |
| [`cxd completion`](commands/completion.md) | Generate Tab completion scripts | Bash, Zsh, Fish, PowerShell |
| [`cxd profile`](commands/run.md) | Show Kata VM resource profiles | JSON output |
| [`cxd default`](configuration.md#project-settings) | Show or change the agent started by `cxd run` | Global config or `--local` |
| `cxd project init/check` | Create and validate repository-local settings | [Configuration](configuration.md#project-settings) |
| [`cxd setup`](../setup.md) | Install, update or restore the node | Plan, --apply, --rollback (Japanese guide) |
| [`cxd init`](../getting-started.md) | Prepare the first worker | Readiness, credentials, image and proxies |
| [`cxd runtime-check`](../troubleshooting.md) | Diagnose execution host | doctor alias, --json and recovery steps |
| [`cxd image check`](../commands/build.md) | Inspect image compatibility | Platform, tools and auth entrypoint |
| [`cxd vm`](../vm.md) | Reconcile and manage Kata VMs | ps / inspect / start / stop / rm / prune |
| [`cxd start`](commands/worker.md) | Restart retained worker | Same ID, restored authentication |
| [`cxd proxy`](commands/proxy.md) | Manage Auth Proxy | build / run / serve / stop / rm |
| [`cxd ps`](commands/worker.md#cxd-ps) | List workers | Show running containers |
| [`cxd stop`](commands/worker.md#cxd-stop) | Stop containers | Single or all |
| [`cxd rm`](commands/worker.md#cxd-rm) | Remove containers | Stopped or forced |
| [`cxd logs`](commands/worker.md#cxd-logs) | View logs | Tail and follow |
| [`cxd auth`](commands/auth.md) | Manage authentication | show / set / rotate |
| [`cxd network`](commands/network-cmd.md) | Manage network | create / rm / status |
| [`cxd firewall`](commands/firewall.md) | (Removed) | Replaced by the router model |
| [`cxd build`](commands/build.md) | Build sandbox image | Dockerfile auto-detection |
| [`cxd ui`](commands/ui.md) | TUI dashboard | Key bindings |
| [`cxd update`](commands/update.md) | Update cxd itself | Update via `go install` |

---

## Global Options

Available on all commands.

| Option | Short | Default | Description |
|---|---|---|---|
| `--verbose` | `-v` | `false` | Enable verbose logging |
| `--debug` | | `false` | Enable debug logging |
| `--config` | | `~/.config/codex-dock/config.toml` | Config file path |

---

## Related Documentation

- [Quick Start](getting-started.md)
- [Configuration Reference](configuration.md)
- [Using Auth Proxy Standalone](proxy-standalone.md)

For administrator-only `node serve` and `runtime-lock`, see [host setup](../kata.md) (Japanese).
