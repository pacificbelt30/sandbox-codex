# Agents

[日本語](../../guides/agents.md) | **English**

- Prerequisites: prepare the [host](../installation.md), [credentials](authentication.md) and worker image.
- Working directory: the project you want the agent to use.

## Supported agents

| Agent | API key | Subscription | Details |
|---|---|---|---|
| Codex CLI | OpenAI API key | ChatGPT OAuth | [Authentication](authentication.md), [launch options](../commands/run.md) |
| Claude Code | Anthropic API key | Claude OAuth or setup-token | [Claude Code](../claude-code.md) |
| Other agents | Not supported | Not supported | [Future work](../future-work.md) |

## Choose a launch method

| Goal | Codex | Claude Code |
|---|---|---|
| Forward native CLI arguments | `oaiso codex` | `oaiso claude` |
| Set image, worktree or mount options | `oaiso run --agent codex` | `oaiso run --agent claude` |
| Set the default for `oaiso run` | `oaiso default codex` | `oaiso default claude` |

- Arguments after `oaiso codex` or `oaiso claude` go to the agent CLI.
- Set Oaiso launch flags through `oaiso run --agent ...`.
- For direct launch, choose the independent profile in [trusted global configuration](../commands/auth.md#fix-the-selection-for-daily-operations).
- The built-in `oaiso run` default is an authenticated shell.
- Project-only defaults: use `oaiso default codex --local`, for example.

## Authentication, network and approval boundaries

| Item | Behavior |
|---|---|
| Real credentials | Stay in the host Auth Proxy; never passed to work VMs |
| Worker AI traffic | Short-lived placeholders authorize fixed AI routes |
| General traffic | Denied by default; [allow required destinations](../../proxy-policy.md) (Japanese) |
| CLI model selection | Selects a model; authorization also requires [proxy model limits](../sandbox-features.md) |
| Agent approvals | Configured with the [run options](../commands/run.md); separate from Kata isolation |

## Next steps

- [File sharing and worktrees](../../workspace.md) (Japanese).
- [List, inspect logs, stop and restart workers](../../operations.md) (Japanese).
- [Update the CLI and images](../../operations.md#cliプロキシを更新する) (Japanese).
- [Run agents through the Go API](../go-api.md).
