# Claude Code

[日本語](../claude-code.md) | **English**

- Run official Claude Code inside a Kata VM using either an Anthropic API key or OAuth from a subscription that includes Claude Code.
- Real credentials stay in the auth proxy.
- The CLI receives only a short-lived worker placeholder.

## API key

Prepare the [execution host](../setup.md), then run from your project:

```bash
export ANTHROPIC_API_KEY='your-api-key'
unset CLAUDE_CODE_OAUTH_TOKEN
oaiso init --agent claude
oaiso claude
# Non-interactive execution
oaiso claude -p 'Explain this project.' --model claude-sonnet-4-6
```

- `oaiso claude` forwards native Claude arguments, including `-p`, `--output-format`, `--resume` and `--model`.
- Configure oaiso image, profile and mounts in global settings or the project's `codex-dock.toml`.
- Existing `oaiso run --agent claude` and `oaiso default claude` remain available.
- `run --task` supplies an initial prompt; use `oaiso claude -p` for non-interactive execution.

## Subscription

Log in with official Claude Code on the host:

```bash
unset ANTHROPIC_API_KEY CLAUDE_CODE_OAUTH_TOKEN
claude auth login
oaiso init --agent claude
oaiso claude
```

- The proxy reads `claudeAiOauth` from host `~/.claude/.credentials.json`.
- Automatic import from macOS Keychain or a custom `CLAUDE_CONFIG_DIR` is not supported.
- If the file is unavailable, run `claude setup-token` on the host, put its output in the host's `CLAUDE_CODE_OAUTH_TOKEN`, then run `oaiso init --agent claude`.
- Keep real tokens out of Dockerfiles, project settings and additional worker environment variables.

- Source precedence is `ANTHROPIC_API_KEY` → `CLAUDE_CODE_OAUTH_TOKEN` → `~/.claude/.credentials.json` → saved Anthropic API key.
- An explicit API key selects API billing; unset it for subscription use.
- Changing host environment variables does not change an already running proxy's authentication.

- Eligibility, available models and usage limits depend on the subscription and Anthropic.
- This integration runs official Claude Code; subscription credentials are not offered as general Anthropic API credentials.
- See [official authentication](https://code.claude.com/docs/en/authentication) and [environment variables](https://code.claude.com/docs/en/env-vars).

## Credential isolation and refresh

| Mode | Worker | Proxy to upstream |
|---|---|---|
| API key | `ANTHROPIC_API_KEY=cdx-…` | Real `x-api-key` |
| Subscription | `CLAUDE_CODE_OAUTH_TOKEN=cdx-…`, empty API key | Real Bearer access token and OAuth beta header |

- `ANTHROPIC_BASE_URL` points to `/anthropic` on the worker network.
- The proxy preserves API version, client beta flags, Messages bodies, tool results and SSE.
- `/token` retrieval is for Codex; Claude uses the environment above.
- Fixed AI routes remain reachable with general egress disabled by `--no-internet`.

- `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1` disables auto-updates, telemetry and other nonessential traffic to keep the CLI pinned.
- Features such as Remote Control are unavailable with this setting.
- Log in and reauthenticate on the host; do not use `/login` inside the VM.

- When the login has a refresh token and expiry, the proxy refreshes within 60 seconds of expiry, with a 20-second request deadline and a 1 MiB response limit.
- Failed refresh/persistence returns 502 without forwarding inference.
- External CredentialStore save failures and version conflicts also revoke the case's tokens and active streams.
- See [auth proxy](auth-proxy.md) and [case credentials](credential-store.md) for durable state and reconnection.

- An access-only `setup-token` credential cannot refresh automatically.
- Reissue it on the host and recreate the proxy after expiry.
- Worker placeholder TTL applies independently, defaults to 3600 seconds, is revoked on stop and restored on managed restart.

## Go API

- Use `Agent: "claude"` with `AgentArgs` to run the CLI as the main process.
- For an idle worker, call the connection helper after `Create` and `Start`:

```go
command, err := client.ClaudeCommand(ctx, worker.ID)
if err != nil {
    return err
}
command.Argv = append(command.Argv, "-p", "Explain this project.", "--model", "claude-sonnet-4-6")
code, err := client.Exec(ctx, worker.ID, cxd.ExecRequest{
    Argv: command.Argv, Env: command.Env, Stdout: output, Stderr: output,
})
```

- `ClaudeCommand` selects the saved API-key/subscription connection and clears competing credential and cloud-provider environment selectors.
- `--setting-sources ""` avoids user/project configuration dependencies.
- Do not log `Env`, which contains a placeholder.
- `Connections` reports the selected variable and header without token values.
- External Store `AnthropicOAuth.RefreshToken` is optional.

- With model restrictions, use exact upstream model IDs.
- Messages and count_tokens support `?beta=true`; other queries, unknown routes and WebSockets are denied.
- Allow any additional models Claude uses for auxiliary requests.
- Do not use `--bare` for subscriptions: official Claude Code does not read OAuth in that mode.

## Update images

- The built-in worker pins Claude Code **2.1.306**.
- Deploy matching CLI/proxy source versions and run from the repository root after ending active workers:

```bash
oaiso build -t codex-dock:latest -f docker/sandbox/Dockerfile
oaiso proxy build
oaiso proxy stop
oaiso proxy rm
oaiso proxy run
```

- Create new workers; existing workers retain their old image and authentication environment.
- Update the entrypoint and Claude version in custom templates too.
- `CODEX_DOCK_CLAUDE_VERSION=2.1.306` explicitly overrides the build version.
- Update and run pinned-CLI acceptance when selecting another version.

See the [verification record](../testing.md#claude-codeの受入) (Japanese) for tested behavior and outstanding live-account checks.
