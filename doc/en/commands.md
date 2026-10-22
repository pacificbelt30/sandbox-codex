# Command Reference

> [日本語](../commands.md) | **English**

Full command reference for codex-dock. See the individual pages below for details on each command.

---

## Command Index

| Command | Description | Details |
|---|---|---|
| [`oaiso claude`](claude-code.md) | Run official Claude Code | Native flags; API or subscription |
| [`oaiso codex`](commands/run.md#launch-codex-cli-directly) | Pass Codex CLI arguments through unchanged | Mount current directory and exec Codex |
| [`oaiso run`](commands/run.md) | Start a sandbox container | Options, approval modes, parallel execution, package management |
| [`oaiso completion`](commands/completion.md) | Generate Tab completion scripts | Bash, Zsh, Fish, PowerShell |
| [`oaiso profile`](commands/run.md) | Show Kata VM resource profiles | JSON output |
| [`oaiso default`](configuration.md#project-settings) | Show or change the agent started by `oaiso run` | Global config or `--local` |
| `oaiso project init/check` | Create and validate repository-local settings | [Configuration](configuration.md#project-settings) |
| [`oaiso setup`](../setup.md) | Install, update or restore the node | Plan, --apply, --rollback (Japanese guide) |
| [`oaiso init`](../getting-started.md) | Prepare the first worker | Readiness, credentials, image and proxies |
| [`oaiso runtime-check`](../troubleshooting.md) | Diagnose execution host | doctor alias, --json and recovery steps |
| [`oaiso image check`](../commands/build.md) | Inspect image compatibility | Platform, tools and auth entrypoint |
| [`oaiso vm`](../vm.md) | Reconcile and manage Kata VMs | ps / inspect / start / stop / rm / prune |
| [`oaiso start`](commands/worker.md) | Restart retained worker | Same ID, restored authentication |
| `oaiso exec` | Run a command in an existing worker | Timeout, stdin, JSON result |
| `oaiso wait` | Wait for a worker to exit | Exit code, timeout, JSON result |
| [`oaiso proxy`](commands/proxy.md) | Manage Auth Proxy | build / run / serve / stop / rm |
| [`oaiso ps`](commands/worker.md#oaiso-ps) | List workers | Show running containers |
| [`oaiso stop`](commands/worker.md#oaiso-stop) | Stop containers | Single or all |
| [`oaiso rm`](commands/worker.md#oaiso-rm) | Remove containers | Stopped or forced |
| [`oaiso logs`](commands/worker.md#oaiso-logs) | View logs | Tail and follow |
| [`oaiso auth`](commands/auth.md) | Manage authentication | show / set / rotate |
| [`oaiso network`](commands/network-cmd.md) | Manage network | create / rm / status |
| [`oaiso firewall`](commands/firewall.md) | (Removed) | Replaced by the router model |
| [`oaiso build`](commands/build.md) | Build sandbox image | Dockerfile auto-detection |
| [`oaiso ui`](commands/ui.md) | TUI dashboard | Key bindings |
| [`oaiso update`](commands/update.md) | Update oaiso itself | Update via `go install` |

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
