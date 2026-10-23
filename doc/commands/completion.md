# `oaiso completion` — Tab 補完

> **日本語** | [English](../en/commands/completion.md)

- `oaiso` は Bash・Zsh・Fish・PowerShell 向けの補完スクリプトを生成できます。
- [配布インストーラー](../installation.md)の `--apply` でBash・Zsh・Fish用の補完も標準のシステムディレクトリへ配置します。
- ソースから導入する場合は `scripts/install.sh --apply` または `make install` でも配置します。
- 新しいシェルを開くと利用できます。
- Bash では `bash-completion` パッケージが必要です。

- 補完の生成・配置対象は `oaiso` のみです。

今のシェルだけで有効にする場合:

```bash
source <(oaiso completion bash)
```

Zsh では `autoload -Uz compinit; compinit` の後で `source <(oaiso completion zsh)`、Fish では `oaiso completion fish | source` を実行します。

- `go install` などで CLI だけを導入した場合は、利用者ごとの補完ディレクトリにも保存できます。
- Bash の例:

```bash
mkdir -p ~/.local/share/bash-completion/completions
oaiso completion bash > ~/.local/share/bash-completion/completions/oaiso
```

- サブコマンドとフラグに加え、`default` のエージェント、`run` のエージェント・runtime・承認モード・テンプレート・`--image`、`start`・`stop`・`rm`・`logs` のワーカー名を補完します。
- `vm inspect/start/stop/rm` では専用ノードの VM 台帳から、コマンドに応じた候補を表示します。
- 通常は VM 名、名前が重複する場合や孤立 VM では ID を使います。
- ワーカー名の取得は最大 500 ms、VM 台帳の取得は最大 2 秒で打ち切ります。
- ノードが停止中でも、ほかの補完は使えます。
