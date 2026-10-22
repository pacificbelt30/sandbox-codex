# 御AgentIsolation (Oaiso) documentation

[日本語](../index.md) | **English**

Oaiso runs Codex CLI and Claude Code in separate Kata VMs.
Containerd manages workers, and Dragonball runs the VMs. Images are built with BuildKit.
Docker Engine, Docker CLI and QEMU are not runtime requirements.

The formal name is **御AgentIsolation**, styled **Oaiso** in English and pronounced “oh-eye-so”. The Japanese prefix 御 adds politeness and respect. The command is `oaiso`; `cxd` and `codex-dock` remain compatibility names. Existing configuration, credentials, service names and Go API identifiers are retained. See [migration compatibility](../migration.md#oaisoへの名称変更) (Japanese).

## Start here

1. An administrator prepares a [Linux/KVM execution host](../kata.md) (Japanese).
2. A user follows the [quick start](getting-started.md) to configure credentials and start a worker.
3. Use the [operations guide](../operations.md) (Japanese) for updates and cleanup.

The execution host must be Linux x86_64. Cross-compiling the CLI does not make Kata available on macOS or Windows.

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
