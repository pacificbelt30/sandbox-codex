# `oaiso upgrade` — update a binary distribution

[日本語](../../commands/upgrade.md) | **English**

- **Development version only; unavailable in published v0.0.2.** On v0.0.2, rerun the [release installer](../../setup.md#更新する).
- No Go installation required. The command reuses the distribution installer and explains host/image update order.

## Review a plan

```bash
oaiso upgrade
oaiso upgrade --version v0.0.2
```

- Choose one. Omit the version to query GitHub's latest stable release.
- Download that release's `install.sh` and `SHA256SUMS` over HTTPS; run the plan only after their hashes match.
- Checksums detect corruption or mismatched assets; they are not independent publisher signatures.
- Planning does not replace product binaries/services. It can prepare uv/Python in user storage and download release assets.

```text
Installed CLI: dev
Target release: v0.0.2

Host update: CLI, node, helpers and pinned runtime assets.
Images: rebuild the default worker after the host update; prepare proxies with the new CLI.
Custom images/templates: review separately.
Before applying: save work, then stop/remove retained workers and proxies with the installed CLI.
...
Plan only. Apply the same release with: oaiso upgrade --version v0.0.2 --apply
```

- Example output; versions and plans depend on your host.
- Applying v0.0.2 or earlier is refused when the selected profile root contains encrypted records. Specify another root explicitly with `--auth-profile-dir`; those older releases cannot read encrypted profiles.

## Apply

1. Review the plan and [release notes](../../release-notes.md).
2. Save work; stop and remove workers/proxies using the installed CLI.
3. Apply the reviewed version:

```bash
oaiso upgrade --version vX.Y.Z --apply
```

- Replace `vX.Y.Z` with the reviewed release tag.
- Run as the normal operator; the installer uses `sudo` for administrative changes.
- Existing installer checks block replacement while retained/live resources or installation inconsistencies remain.
- See [setup](../../setup.md) for recovery.

## Scope and order

| Order | Target | Action |
|---|---|---|
| 1 | CLI, node, guard, IO logger and pinned runtime assets | Distribution installer through `upgrade --apply` |
| 2 | Default worker image | New CLI: `oaiso build --template plain --tag codex-dock:latest` |
| 3 | Authentication proxy | New CLI: `oaiso init --agent codex` or `--agent claude` |
| 4 | Custom images/templates | Review and rebuild individually |
| 5 | Behavior | Verify connections/inference in a new worker |

- Images are rebuilt with the newly installed CLI, not the old CLI still in memory.
- [`oaiso update`](update.md) separately updates only a Go-installed CLI.
