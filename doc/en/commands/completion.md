# `oaiso completion` — shell completion

> [日本語](../../commands/completion.md) | **English**

- `oaiso` can generate completion scripts for Bash, Zsh, Fish, and PowerShell.
- `scripts/install.sh --apply` and `make install` place Bash, Zsh, and Fish scripts in the standard system directories.
- Open a new shell to use them.
- Bash requires the `bash-completion` package.

- Completion is generated and installed only for `oaiso`.

For the current Bash session:

```bash
source <(oaiso completion bash)
```

- For Zsh, run `autoload -Uz compinit; compinit` before `source <(oaiso completion zsh)`.
- For Fish, run `oaiso completion fish | source`.

- If you installed only the CLI with `go install`, you can save the script in your user's completion directory.
- For Bash:

```bash
mkdir -p ~/.local/share/bash-completion/completions
oaiso completion bash > ~/.local/share/bash-completion/completions/oaiso
```

- Completion covers commands and flags, agent choices for `default`, agent/runtime/approval/template/`--image` values for `run`, and worker names for `start`, `stop`, `rm`, and `logs`.
- For `vm inspect/start/stop/rm`, candidates come from the dedicated node's VM inventory and reflect the action.
- Completion uses VM names when unambiguous and IDs for duplicate names or orphans.
- Worker lookups have a 500 ms deadline; VM inventory lookups have a 2 second deadline.
- Other completion remains available if the node is stopped.
