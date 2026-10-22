# 御AgentIsolation (Oaiso) documentation

[日本語](../index.md) | **English**

| Item | At a glance |
|---|---|
| Purpose | Run Codex CLI and Claude Code in separate Kata VMs |
| Command | `oaiso` |
| Runtime | Containerd manages workers; Dragonball runs VMs |
| Images | Built with BuildKit |
| Host | Linux x86_64; Docker Engine, Docker CLI and QEMU are not runtime requirements |

## Start here

1. Follow [Installation](installation.md) to install the CLI and execution host, then verify the setup.
2. A user follows the [quick start](getting-started.md) to configure credentials and start a worker.
3. Use the [operations guide](../operations.md) (Japanese) for updates and cleanup.

- The execution host must be Linux x86_64.
- Cross-compiling the CLI does not make Kata available on macOS or Windows.

## Find a topic

| Task | Guide |
|---|---|
| Run Claude Code with API or subscription auth | [Claude Code](claude-code.md) |
| Move from Docker | [Migration](../migration.md) (Japanese) |
| Understand shared files and ownership | [Workspace](../workspace.md) (Japanese) |
| Inspect and manage VMs | [VM management](../vm.md) (Japanese) |
| Build a custom image | [Build command](commands/build.md) |
| Restrict outbound traffic | [Network](network.md), [policy rules](../proxy-policy.md) (Japanese) |
| Understand credentials | [Auth proxy](auth-proxy.md) |
| Use the proxy by itself | [Standalone proxy](proxy-standalone.md) |
| Look up flags and settings | [Commands](commands.md), [configuration](configuration.md) |
| Manage workers from a host product | [Go API](go-api.md) |
| Measure performance and compare revisions | [Performance and benchmarks](performance.md) |
| Diagnose a failure | [Troubleshooting](../troubleshooting.md) (Japanese) |
| Understand the design | [Architecture](architecture.md), [security](security.md) |
| Check test coverage | [Testing](../testing.md) (Japanese) |
| See planned coding agent integrations | [Future work](future-work.md) |

[Additional sandbox controls](sandbox-features.md)

## Name and compatibility

| Item | Meaning or compatibility |
|---|---|
| Formal name | 御AgentIsolation; 御 adds politeness and respect |
| English name / pronunciation | Oaiso / “oh-eye-so” |
| Legacy commands | `cxd` / `codex-dock` |
| Retained identifiers | Configuration, credentials, service names and Go API |
| Details | [Migration compatibility](../migration.md#oaisoへの名称変更) (Japanese) |
