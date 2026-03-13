# `codex-dock auth` — Authentication Management

> [日本語](../../commands/auth.md) | **English**
>
> [← Command Reference](../commands.md)

---

## `auth show`

```bash
codex-dock auth show
```

Displays the current auth source (actual keys/tokens are not shown).

---

## `auth set`

```bash
export OPENAI_API_KEY=sk-...
codex-dock auth set
```

Saves `OPENAI_API_KEY` to `~/.config/codex-dock/apikey` with permissions `0600`.

---

## `auth rotate`

```bash
codex-dock auth rotate
```

Immediately revokes all currently active tokens.

---

## Related Documentation

- [Auth Proxy Specification](../auth-proxy.md)
- [Configuration Reference](../configuration.md)
- [Quick Start](../getting-started.md)
