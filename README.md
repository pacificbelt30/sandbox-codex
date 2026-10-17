# codex-dock

Codex CLIとClaude Codeを、KataのVMで隔離して実行するツールです。
ワーカーごとに独立したカーネルを使い、認証情報と外部通信はプロキシで管理します。

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

1. 管理者が[Kata実行ホストを準備](doc/kata.md)します。
2. 利用者が[クイックスタート](doc/getting-started.md)に沿って認証とイメージを用意します。
3. ワーカーを起動します。

準備済みのホストでは、認証を設定してから次の順序で実行します。
ChatGPT認証を使う場合は、同じ利用者の `~/.codex/auth.json` を参照します。
APIキーを使う場合は、先に `OPENAI_API_KEY` または `ANTHROPIC_API_KEY` を設定してください。

```bash
codex-dock runtime-check
codex-dock proxy build
codex-dock proxy run
codex-dock build

codex-dock run --agent codex
# または
codex-dock run --agent claude
```

`--agent` を省略すると、認証設定済みのシェルが開きます。
通常の操作はoperator groupに属する利用者で行います。
Kataの検証に失敗した場合は起動を中止し、runcへ自動で切り替えません。

## よく使う操作

```bash
# 独立したブランチでバックグラウンド実行
codex-dock run --agent codex --task "テストを追加して" \
  --worktree --branch add-tests --new-branch --detach

codex-dock ps -a
codex-dock vm ps -a
codex-dock logs WORKER --follow
codex-dock stop WORKER
codex-dock vm start WORKER
codex-dock stop WORKER
codex-dock rm WORKER
codex-dock ui
```

`ps` はワーカーの状態、`vm ps` は台帳・containerd・ホストプロセスを照合したVMの状態を表示します。
停止したワーカーは再起動用に残ります。前面で実行したワーカーは、既定では終了時に削除されます。
残したい場合は `run --keep` を指定します。

## 読みたいことから探す

| 目的 | ドキュメント |
|---|---|
| 初めて使う | [クイックスタート](doc/getting-started.md) |
| 実行ホストを用意する | [Kata / Dragonballの配置](doc/kata.md) |
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
