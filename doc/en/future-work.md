# Future work

In addition to Codex CLI and Claude Code, the following coding agents are candidates for future integration. They are not currently supported. Adoption, implementation order and release dates are undecided.

## Additional coding agents

| Agent | Integration work to investigate and implement |
|---|---|
| [Gemini CLI](https://github.com/google-gemini/gemini-cli) | Launch adapter, authentication, Gemini proxy routing and headless execution |
| [GitHub Copilot CLI](https://github.com/github/copilot-cli) | Copilot authentication and routing, GitHub integration, interactive and non-interactive execution |
| [OpenCode](https://opencode.ai/docs/) | Launch adapter, provider configuration, proxy routing and non-interactive execution |
| [Aider](https://aider.chat/) | API/proxy configuration, Git changes and commits, non-interactive execution |
| [Cursor CLI](https://cursor.com/docs/cli/overview) | Launch adapter, authentication, endpoint configuration and headless execution |
| [Cline CLI](https://docs.cline.bot/usage/cli-overview) | Launch adapter, provider/auth configuration, headless execution and tools |
| [Kilo Code CLI](https://kilo.ai/docs/code-with-ai/platforms/cli) | Launch adapter, provider/auth configuration and proxy routing |
| [Goose](https://github.com/aaif-goose/goose) | Launch adapter, provider/auth configuration and tool/MCP integration |

Investigate local CLI execution inside Kata separately from editor extensions and hosted offerings. API and subscription authentication must be assessed per agent/provider; support for both is not assumed.

## Shared acceptance conditions

- Retain the authentication proxy. Never pass real API keys, access tokens, refresh tokens or ID tokens to work VMs. Workers receive only short-lived, per-run, proxy-scoped placeholders.
- Use independent product authentication profiles, without automatically importing host CLI login files or inherited credentials. Hold integrations whose storage or proxy use cannot satisfy official authentication terms.
- Login-only Kata sessions must not persist secrets; discard temporary state on completion, cancellation and failure. Do not replace the proxy with a persistent VM holding real credentials.
- Prioritize independent credential management. Real ID-token passthrough has been removed; do not weaken credential isolation or model authorization to add an agent.
- Pin CLI versions, distributions and images; provide explicit launch settings independent of user/project configuration. Extend [static upstream checks](upstream-compatibility.md) for each agent.
- Verify startup, tools, streaming, cancellation, stop/restart, revocation and credential leakage into logs, files and snapshots in real Kata. Record verified behavior and unsupported constraints in the [testing guide](../testing.md).
- Review CLI/SDK licenses, retained baseline data, distribution terms and authentication/subscription conditions before integration.
