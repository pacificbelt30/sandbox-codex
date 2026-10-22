# Authentication and profiles

[日本語](../../guides/authentication.md) | **English**

- Prerequisite: complete [installation](../installation.md).
- Configure authentication on the trusted host, including a WSL or VM host.

## 1. Choose credential separation

| Mode | Credential storage | Use it for |
|---|---|---|
| Independent profile | Product directory with mode `0700`; files with mode `0600` | Separate product authentication from host CLI logins |
| Legacy host integration | Host credential files and authentication environment variables | Reuse existing host authentication |
| Embedding product Store | Storage provided by the embedding application | Manage per-case credentials through the [Go API](../credential-store.md) |

- Selected independent profiles never fall back to host authentication.
- Current independent profile storage: permission-restricted plaintext.
- Keep credentials and profile selection out of project `codex-dock.toml` files.

## 2. Choose the agent and authentication method

| Agent | API key | Subscription |
|---|---|---|
| Codex CLI | [Register](../commands/auth.md#register-an-api-key) an OpenAI key | [Import](../commands/auth.md#import-oauth) Codex OAuth credentials |
| Claude Code | [Register](../commands/auth.md#register-an-api-key) an Anthropic key | [Import](../commands/auth.md#import-oauth) Claude OAuth credentials; see [Claude setup-token](../claude-code.md) |

- `auth profile set`: hidden TTY input; keep real keys out of command arguments.
- `auth profile import`: explicit stdin input; source files are not automatically discovered or deleted.
- Legacy host integration: [prepare credentials](../getting-started.md#2-prepare-credentials).

## 3. Configure admin authority and profile selection

1. [Create the admin secret](../commands/auth.md#create-admin-authority-and-start-the-proxy) in a separate `0600` file.
2. [Select the profile globally](../commands/auth.md#fix-the-selection-for-daily-operations).
3. From the source root, run `oaiso init` or `oaiso init --agent claude`.
4. [Launch the agent](agents.md).

## Refresh and switching

| Operation | Behavior |
|---|---|
| Normal OAuth refresh | CAS save to the same Store; retain the connection generation |
| Re-login or account switch | Import into a new profile, then [replace the workers and proxy](../commands/auth.md#refresh-switching-and-sharing) |
| Credentials passed to workers | Short-lived placeholders only; no real API key or access/refresh/ID token |
| Dedicated login Kata, automated login and live API-key validation | Not implemented |
| Encrypted vault and key backends | Not implemented; see [design requirements](../credential-store.md) |

- Storage, permissions and limitations: [Security](../security.md).
- Flags and file paths: [Authentication command reference](../commands/auth.md).
