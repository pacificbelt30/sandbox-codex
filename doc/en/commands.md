# Command Reference

> [日本語](../commands.md) | **English**

Full command reference for codex-dock. See the individual pages below for details on each command.

---

## Command Index

| Command | Description | Details |
|---|---|---|
| [`codex-dock run`](commands/run.md) | Start a sandbox container | Options, approval modes, parallel execution, package management |
| [`codex-dock proxy`](commands/proxy.md) | Manage Auth Proxy | build / run / serve / stop / rm |
| [`codex-dock ps`](commands/worker.md#codex-dock-ps) | List workers | Show running containers |
| [`codex-dock stop`](commands/worker.md#codex-dock-stop) | Stop containers | Single or all |
| [`codex-dock rm`](commands/worker.md#codex-dock-rm) | Remove containers | Stopped or forced |
| [`codex-dock logs`](commands/worker.md#codex-dock-logs) | View logs | Tail and follow |
| [`codex-dock auth`](commands/auth.md) | Manage authentication | show / set / rotate |
| [`codex-dock network`](commands/network-cmd.md) | Manage network | create / rm / status |
| [`codex-dock firewall`](commands/firewall.md) | Manage firewall | create / status / rm |
| [`codex-dock build`](commands/build.md) | Build sandbox image | Dockerfile auto-detection |
| [`codex-dock ui`](commands/ui.md) | TUI dashboard | Key bindings |

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
