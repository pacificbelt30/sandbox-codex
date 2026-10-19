# `cxd completion` — Tab 補完

> **日本語** | [English](../en/commands/completion.md)

`cxd` は Bash・Zsh・Fish・PowerShell 向けの補完スクリプトを生成できます。`scripts/install.sh --apply` と `make install` は、Bash・Zsh・Fish 用のスクリプトを標準のシステムディレクトリに配置します。新しいシェルを開くと利用できます。Bash では `bash-completion` パッケージが必要です。

今のシェルだけで有効にする場合:

```bash
source <(cxd completion bash)
```

Zsh では `autoload -Uz compinit; compinit` の後で `source <(cxd completion zsh)`、Fish では `cxd completion fish | source` を実行します。

`go install` などで CLI だけを導入した場合は、利用者ごとの補完ディレクトリにも保存できます。Bash の例:

```bash
mkdir -p ~/.local/share/bash-completion/completions
cxd completion bash > ~/.local/share/bash-completion/completions/cxd
```

サブコマンドとフラグに加え、`default` のエージェント、`run` のエージェント・runtime・承認モード・テンプレート・`--image`、`start`・`stop`・`rm`・`logs` のワーカー名を補完します。ワーカー名は専用ノードから最大 500 ms で取得します。ノードが停止中でも、ほかの補完は使えます。
