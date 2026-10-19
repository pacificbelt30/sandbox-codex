# cxd

Codex CLIとClaude Codeを、KataのVMで隔離して実行するツールです。
ワーカーごとに独立したカーネルを使い、認証情報と外部通信はプロキシで管理します。
コマンド名は `cxd` です。既存の `codex-dock` コマンドも引き続き使えます。設定ファイルやサービス名は従来の名前を維持します。

実行には **Linux x86_64・KVM・containerd・Kata runtime-rs・Dragonball** が必要です。
イメージはBuildKitで作成します。Docker Engine、Docker CLI、QEMUは実行時に使いません。

[日本語ドキュメント](doc/index.md) · [English documentation](doc/en/index.md)

## できること

- **VM隔離**：ワーカーごとにCPU・メモリ・プロセス数を制限します。
- **認証の分離**：実APIキーやOAuthのaccess token・refresh tokenは認証プロキシで扱います。ワーカーには短命トークンを渡します。
- **通信の制限**：ワーカー間・ホスト・外部への直接通信を遮断します。一般通信にはドメイン・IP・ポートの許可／拒否ルールを設定できます。
- **並列開発**：Git worktreeで作業場所とブランチを分けられます。
- **イメージの拡張**：Dockerfile、テンプレート、追加パッケージを使えます。
- **管理**：CLIとTUIで一覧・ログ・停止・再起動・削除を操作できます。

## 最初に使う

1. 管理者が[実行ホストを準備](doc/setup.md)します。
2. 利用者が[クイックスタート](doc/getting-started.md)に沿って認証とイメージを用意します。
3. ワーカーを起動します。

Ubuntu 26.04 の対応ホストでは、ソースのルートから `./scripts/install.sh` で導入計画を確認し、`./scripts/install.sh --apply` でバンドル作成から適用まで実行できます。

準備済みのホストでは、認証を設定してから次の順序で実行します。
ChatGPT認証を使う場合は、同じ利用者の `~/.codex/auth.json` を参照します。
APIキーを使う場合は、先に `OPENAI_API_KEY` または `ANTHROPIC_API_KEY` を設定してください。

```bash
cxd init
cxd codex

# Codex CLI の引数もそのまま渡せる
cxd codex "このリポジトリを調べて"

# Claude Codeを使う場合
cxd init --agent claude
cxd run --agent claude
```

プロキシイメージを初めてビルドする際は、リポジトリのルートで `init` を実行します。
`cxd run` は組み込み既定値では認証済みシェルを開きます。`cxd default codex` で既定を Codex に、`cxd default codex --local` でこのプロジェクトだけ Codex に変更できます。`cxd default` で有効な値を確認できます。
導入スクリプトと `make install` は Bash・Zsh・Fish の Tab 補完も配置します。[手動で有効にする方法](doc/commands/completion.md)もあります。
`cxd codex` はカレントディレクトリを `/workspace` にマウントし、Codex CLI を直接起動します。コンテナの細かい指定には `run --agent codex` を使います。
通常の操作はoperator groupに属する利用者で行います。
Kataの検証に失敗した場合は起動を中止し、runcへ自動で切り替えません。

## よく使う操作

```bash
# 独立したブランチでバックグラウンド実行
cxd run --agent codex --task "テストを追加して" \
  --worktree --branch add-tests --new-branch -d

cxd ps -a
cxd vm ps -a
cxd logs WORKER --follow
cxd stop WORKER
cxd start WORKER
cxd rm WORKER
cxd ui
```

`ps` はワーカーの状態、`vm ps` は台帳・containerd・ホストプロセスを照合したVMの状態を表示します。
停止したワーカーは再起動用に残ります。前面で実行したワーカーは、既定では終了時に削除されます。
残したい場合は `run --keep` を指定します。

## プロジェクトごとの設定

リポジトリのルートで `cxd project init` を実行すると、コミット可能な `codex-dock.toml` ができます。`cxd project check` で内容を検証できます。クローン先ではホストのセットアップと認証を済ませ、リポジトリのルートで `cxd init`、`cxd codex` を実行します。

プロジェクト設定にはイメージ、ビルド元、モデルなどを記述します。`packages.dock` もコミットすれば追加パッケージを共有できます。認証情報、containerd/Kataの接続先、プロキシ、追加ホストマウントはグローバル設定または明示したCLI引数で管理します。[設定例と優先順位](doc/configuration.md#プロジェクト設定)を参照してください。

## 読みたいことから探す

| 目的 | ドキュメント |
|---|---|
| 初めて使う | [クイックスタート](doc/getting-started.md) |
| 実行ホストを用意・更新する | [自動セットアップ](doc/setup.md) / [手動配置](doc/kata.md) |
| 停止・再起動・更新する | [日常の運用](doc/operations.md) |
| 旧Docker環境から移る | [移行手順](doc/migration.md) |
| 共有ファイル・所有者を確認する | [ファイル共有](doc/workspace.md) |
| VMの状態を調べる | [VM管理](doc/vm.md) |
| 独自イメージを作る | [buildコマンド](doc/commands/build.md) |
| 外部通信を制限する | [通信ポリシー](doc/proxy-policy.md) |
| 設定・オプションを調べる | [設定](doc/configuration.md)・[コマンド](doc/commands.md) |
| 問題を調べる | [トラブルシューティング](doc/troubleshooting.md) |
| 仕組みと確認範囲を知る | [構成](doc/architecture.md)・[セキュリティ](doc/security.md)・[検証](doc/testing.md) |

## 開発

ソースのビルドにはGo 1.24.7以上とMakeが必要です。
`make build` はCLI、ワーカー起動用guard、IO loggerを作成します。
ホストへの配置は[Kataの手順](doc/kata.md)を参照してください。

```bash
make build
make test
make vet
```

設定ファイルは `~/.config/codex-dock/config.toml` です。
記述例は [configs/config.toml.example](configs/config.toml.example) にあります。

VMに共有したworkspaceや追加マウントは、指定した権限で変更できます。
管理ソケットとBuildKitを使えるホスト利用者はroot相当の権限を持ちます。
保護の範囲と制限は[セキュリティ設計](doc/security.md)を参照してください。
