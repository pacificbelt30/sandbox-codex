# `cxd completion` — shell completion

> [日本語](../../commands/completion.md) | **English**

`cxd` can generate completion scripts for Bash, Zsh, Fish, and PowerShell. `scripts/install.sh --apply` and `make install` place Bash, Zsh, and Fish scripts in the standard system directories. Open a new shell to use them. Bash requires the `bash-completion` package.

For the current Bash session:

```bash
source <(cxd completion bash)
```

For Zsh, run `autoload -Uz compinit; compinit` before `source <(cxd completion zsh)`. For Fish, run `cxd completion fish | source`.

If you installed only the CLI with `go install`, you can save the script in your user's completion directory. For Bash:

```bash
mkdir -p ~/.local/share/bash-completion/completions
cxd completion bash > ~/.local/share/bash-completion/completions/cxd
```

Completion covers commands and flags, agent choices for `default`, agent/runtime/approval/template/`--image` values for `run`, and worker names for `start`, `stop`, `rm`, and `logs`. Worker names are fetched from the dedicated node with a 500 ms deadline. Other completion remains available if the node is stopped.
