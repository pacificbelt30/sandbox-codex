# Auth proxy

[日本語](../auth-proxy.md) | **English**

The auth proxy validates a worker's short-lived token, then injects real credentials when sending a request to a fixed AI upstream.
OpenAI and Anthropic credentials are loaded independently.

## Two roles

| Proxy | Purpose | Real credentials |
|---|---|---|
| `codex-auth-proxy` | AI reverse routes, token API, admin API | Yes |
| `codex-http-proxy` | General HTTP/CONNECT traffic such as Git, npm and pip | No |

General egress rules do not control the fixed AI routes.

## Start

Prepare the [Kata host](../kata.md) and credentials first. Build from the repository root.

```bash
cxd proxy build
cxd proxy run
```

Both proxies run as runc containers on containerd. Workers use Kata VMs.

| Caller | Default endpoint |
|---|---|
| Host CLI / admin | `http://127.0.0.1:18081` |
| Worker / AI data | `http://codex-auth-proxy:18080` |
| Worker / general egress | `http://codex-http-proxy:18082` |

Worker proxy names are pinned in static hosts entries on private networks.
The separate admin listener is not exposed to workers.

## Credential sources

Each provider uses the first available source below.

| Priority | OpenAI | Anthropic |
|---|---|---|
| 1 | `~/.codex/auth.json` OAuth | `ANTHROPIC_API_KEY` |
| 2 | `OPENAI_API_KEY` | `CLAUDE_CODE_OAUTH_TOKEN` |
| 3 | `~/.config/codex-dock/apikey` | `~/.claude/.credentials.json` OAuth |
| 4 | `~/.codex/auth.json` API key | `~/.config/codex-dock/anthropic-apikey` |

An explicit Anthropic API key selects API billing even with a saved subscription login. Unset it to use subscription OAuth. See [Claude Code](claude-code.md) for setup-token support, refresh and pinned-CLI execution. External case proxies use only their CredentialStore, without these host fallbacks.

The auth proxy runs as the caller's UID/GID with a private HOME at `/var/lib/codex-dock-proxy`.
Credential files are mounted read-only. Owner-only `0600` files work without permission changes or added capabilities.
Unreadable files and directories are rejected before startup.

## Request flow

1. The host issues a short-lived token for a worker.
2. The entrypoint retrieves placeholder credentials and client metadata from `/token`.
3. The agent calls an AI route with the placeholder.
4. The proxy validates the token before connecting upstream.
5. It replaces worker authentication with the real provider credentials.

Real API keys and OAuth access/refresh/ID tokens never reach workers.
ChatGPT mode creates a fresh dummy JWT containing only account ID, plan and the FedRAMP flag. The worker's auth.json contains this dummy ID JWT, a placeholder access token and an empty refresh token.

## Authentication and expiry

| Route | Worker authentication |
|---|---|
| `/v1/*`, `/chatgpt/*` | `Authorization: Bearer <worker token>` |
| `/anthropic/*` | `x-api-key: <worker token>` or Bearer |

HTTP and WebSocket requests use the same checks.
Missing, unissued, expired, revoked, duplicate or conflicting credentials return 401 before an upstream connection.
Revocation, expiry and token replacement also cancel active HTTP streams and WebSocket tunnels.
Tokens default to 3600 seconds, are revoked on stop and restored on managed restart.
See [tokens](auth-proxy/tokens.md).

Token values use a direct lookup index, updated under the same lock as issuance, restore and revocation. Authentication no longer scans every worker. Expiry, provider/model restrictions and active-stream cancellation remain enforced. See the [performance measurements](../testing.md#2026-10-21の性能計測) (Japanese).

## Upstream authentication

| Mode | Upstream | Injected credentials |
|---|---|---|
| OpenAI API key | `api.openai.com/v1/` | Bearer API key |
| ChatGPT OAuth | `chatgpt.com/backend-api/codex/` | Bearer access token and account ID |
| Anthropic API key | `api.anthropic.com/` | `x-api-key` |
| Anthropic OAuth | `api.anthropic.com/` | Bearer access token and OAuth beta header |

OpenAI refresh uses `/oauth/token`. Anthropic refresh is handled by the proxy when needed.
Real refresh tokens stay on the proxy side. See [verification limits](../testing.md) (Japanese).

## Updates and standalone use

Rebuild and recreate proxies when updating the CLI; building an image alone does not update running containers.
A proxy without `worker_token_auth: true` is rejected by the current CLI.
See [operations](../operations.md) (Japanese).

See [standalone use](proxy-standalone.md), [API reference](auth-proxy/endpoints.md) and [extension boundaries](../proxy-policy.md#コードの境界) (Japanese).
