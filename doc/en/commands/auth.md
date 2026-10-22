# `cxd auth` — Authentication Management

> [日本語](../../commands/auth.md) | **English**
>
> [← Command Reference](../commands.md)

## Independent profiles

Explicitly select a product profile to keep authentication separate from host CLI logins. Selected profiles never fall back to host environment variables or login files. Missing profiles and unsafe permissions fail.

```bash
cxd auth profile set work --provider openai  # hidden TTY input
cxd auth profile set claude-work --provider anthropic
cxd auth profile show work
cxd auth profile list
```

Noninteractive input requires `--key-stdin`; supply it from a protected source, never put the key in arguments. Storage is plaintext at `~/.config/codex-dock/profiles/work/work.json`, with `0700` root/profile directories and `0600` JSON/lock files. Override the root using `--auth-profile-dir` or trusted global `auth_profile_dir`. In WSL use private Linux-side storage, outside shared or synced folders.

OAuth import also requires explicit stdin:

```bash
cxd auth profile import work-oauth --format codex --credentials-stdin < /private/product-login/auth.json
cxd auth profile import claude-oauth --format claude --credentials-stdin < /private/product-login/.credentials.json
```

`codex` reads native `tokens` and `last_refresh`; `claude` reads `claudeAiOauth`. The default `credentials` format is `credentialstore.Credentials` JSON and can include both providers. Source files are neither discovered nor deleted automatically. Automated login-only Kata and live API-key validation are not implemented.

Use a separate `0600` file for admin authority to avoid secret values in process arguments. This example refuses to overwrite an existing file:

```bash
python3 - <<'PY'
import os, pathlib, secrets
path = pathlib.Path.home() / '.config/codex-dock/proxy-admin-secret'
with os.fdopen(os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600), 'w') as f:
    f.write(secrets.token_hex(32) + '\n')
PY
cxd --auth-profile work proxy run --admin-secret-file "$HOME/.config/codex-dock/proxy-admin-secret"
cxd --auth-profile work run --agent codex --proxy-admin-secret-file "$HOME/.config/codex-dock/proxy-admin-secret"
```

For `init`, `run`, `start` and TUI lifecycle management, put selection and the secret-file path in trusted global `~/.config/codex-dock/config.toml`, outside project configuration:

```toml
auth_profile = "work"
[proxy]
admin_secret_file = "/home/user/.config/codex-dock/proxy-admin-secret"
```

Native `cxd codex` / `cxd claude` forward arguments to the agent CLI; select profiles through global configuration there. `cxd run` accepts the flags above. Profile-selected `auth show` displays redacted metadata.

Creation/import never overwrites a profile. Import re-logins under a new name, stop/remove old workers, then explicitly recreate the proxy before switching. Retained workers cannot be reauthorized under a different profile or login generation. Normal OAuth refresh uses CAS and preserves the connection generation. Only the selected directory is mounted read-write into the auth proxy; neither workers nor egress receive it. Keep credential storage and admin-secret files outside workspaces and extra mounts.

The following legacy commands apply without a selected profile. Profile mode rejects legacy `auth set` / `auth rotate`.

---

## `auth show`

```bash
cxd auth show
```

Displays the current auth source (actual keys/tokens are not shown).

---

## `auth set`

```bash
export OPENAI_API_KEY=sk-...
cxd auth set
```

Saves `OPENAI_API_KEY` to `~/.config/codex-dock/apikey` with permissions `0600`.

---

## `auth rotate`

```bash
cxd auth rotate
```

The current implementation only writes a local `.rotate` marker. It does not immediately revoke tokens in a running proxy.
Stop the worker or use `/admin/revoke` for immediate revocation. See [operations](../../operations.md) (Japanese) for proxy updates.

---

## Related Documentation

- [Auth Proxy Specification](../auth-proxy.md)
- [Configuration Reference](../configuration.md)
- [Quick Start](../getting-started.md)
