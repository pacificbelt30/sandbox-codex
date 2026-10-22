# 御AgentIsolation（Oaiso）

**Codex CLI・Claude Codeを、ワーカーごとのKata VMで隔離して実行。**

| 項目 | 内容 |
|---|---|
| 正式名称 | 御AgentIsolation。「御」は丁寧さ・敬意を添える接頭辞 |
| 英字表記・読み | Oaiso／おあいそ |
| コマンド | `oaiso`。旧 `cxd` / `codex-dock` も利用可 |
| 実行ホスト | Linux x86_64・KVM・containerd・Kata runtime-rs・Dragonball |
| イメージ作成 | BuildKit |
| 実行時に不要 | Docker Engine・Docker CLI・QEMU |

[日本語ドキュメント](doc/index.md) · [English documentation](doc/en/index.md)

## できること

| 目的 | 機能・参照先 |
|---|---|
| エージェントを動かす | Codex CLI・[Claude Code](doc/claude-code.md)。ClaudeはAPIキー／サブスクに対応 |
| 作業を隔離する | ワーカーごとに独立したカーネル。CPU・メモリ・プロセス数を制限 |
| 認証情報を分ける | 実資格情報は認証Proxyで保持。ワーカーには短命トークンを渡す |
| 通信を制限する | 直接の外部・ホスト・ワーカー間通信を遮断。一般通信はドメイン・IP・ポートで制御 |
| 並列で開発する | VMとGit worktreeを分けて実行 |
| 実行環境を拡張する | Dockerfile・テンプレート・追加パッケージ |
| 状態を管理する | CLI／TUIで一覧・ログ・停止・再起動・削除 |
| 製品に組み込む | 同じホストの製品から[Go API](doc/go-api.md)で操作 |

## 最初に使う

| 順序 | 担当 | 実施内容 |
|---|---|---|
| 1 | 管理者 | [実行ホストを準備](doc/setup.md)。Ubuntu 26.04対応ホストでは `./scripts/install.sh` で計画、`--apply` で適用 |
| 2 | 利用者 | [認証とイメージを準備](doc/getting-started.md)。ホストと分ける場合は[独立プロファイル](doc/commands/auth.md#独立プロファイル)を選択 |
| 3 | 利用者 | プロジェクトでワーカーを起動 |

**従来のホスト認証を使う場合**

| 認証方式 | 準備するもの |
|---|---|
| ChatGPT認証 | 同じ利用者の `~/.codex/auth.json` |
| APIキー | `OPENAI_API_KEY` または `ANTHROPIC_API_KEY` |

```bash
oaiso init
oaiso codex

# Codex CLI の引数もそのまま渡せる
oaiso codex "このリポジトリを調べて"

# Claude Codeを使う場合
oaiso init --agent claude
oaiso run --agent claude
```

**起動時の要点**

- 初回のProxyイメージ作成：リポジトリのルートで `init` を実行。
- 通常操作：operator groupの利用者で実行。
- `oaiso run` の組み込み既定値：認証済みシェル。
- Codexを既定にする：`oaiso default codex`。
- プロジェクト限定は `--local`。
- 現在の既定値を確認：`oaiso default`。
- `oaiso codex`：現在のディレクトリを `/workspace` に共有し、Codexを直接起動。
- コンテナの詳細指定：`oaiso run --agent codex`。
- Kata検証失敗時：起動中止。
- runcへ自動切り替えしない。
- Tab補完：導入スクリプト／`make install` がBash・Zsh・Fish用を配置。
- [手動設定](doc/commands/completion.md)も可。

## よく使う操作

```bash
# 独立したブランチでバックグラウンド実行
oaiso run --agent codex --task "テストを追加して" \
  --worktree --branch add-tests --new-branch -d

oaiso ps -a
oaiso vm ps -a
oaiso logs WORKER --follow
oaiso stop WORKER
oaiso start WORKER
oaiso rm WORKER
oaiso ui
```

| 操作 | 対象・終了後の扱い |
|---|---|
| `ps` | ワーカーの状態 |
| `vm ps` | 台帳・containerd・ホストプロセスを照合したVMの状態 |
| `stop` | 再起動用にワーカーを保持 |
| 前面実行 | 終了時に削除。保持する場合は `run --keep` |

## プロジェクトごとの設定

1. リポジトリのルートで `oaiso project init`。
2. 生成した `codex-dock.toml` をコミット。
3. クローン先でホストと認証を準備。
4. `oaiso project check` → `oaiso init` → `oaiso codex`。

| 設定先 | 書く内容 |
|---|---|
| `codex-dock.toml` | イメージ・ビルド元・モデルなど。独自環境はプロジェクトのDockerfileを `[build]` で指定 |
| `packages.dock` | チームで共有する追加パッケージ |
| グローバル設定／明示CLIフラグ | 認証情報・containerd/Kata接続先・Proxy・追加ホストマウント |

[設定例と優先順位](doc/configuration.md#プロジェクト設定)

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
| 製品にワーカー管理を組み込む | [Go API](doc/go-api.md) |
| 性能値・再計測手順を確認する | [性能とベンチマーク](doc/performance.md)・[計測結果](doc/testing.md#2026-10-21の性能計測) |
| 問題を調べる | [トラブルシューティング](doc/troubleshooting.md) |
| 仕組みと確認範囲を知る | [構成](doc/architecture.md)・[セキュリティ](doc/security.md)・[検証](doc/testing.md) |

## 開発

- 必要なツール：Go 1.24.7以上・Make。
- `make build`：CLI・ワーカー起動用guard・IO loggerを作成。
- ホストへの配置：[Kataの手順](doc/kata.md)。

```bash
make build
make test
make vet
```

### ドキュメントの編集方針

- ページの冒頭：目的・前提条件・最初の操作を示す。
- 操作の順序：番号付き手順と実行コマンドで示す。
- 選択肢・設定・既定値・制限：表で比較する。
- 補足事項：1項目1要点の短い箇条書きにする。
- 詳細仕様・検証記録：専用ページへ分け、利用手順からリンクする。

## 引き継ぐ設定と保護の範囲

| 項目 | 扱い |
|---|---|
| グローバル設定 | `~/.config/codex-dock/config.toml`。[記述例](configs/config.toml.example) |
| 名称変更後の互換性 | 設定・認証情報の保存先、サービス・イメージ名、Go module・`pkg/cxd` を維持 |
| 従来モードのOAuth更新 | `~/.local/state/codex-dock/oauth/` に0700/0600で保存。Proxy再起動後も復元 |
| 再ログイン | 元の認証ファイルが変わると、以前のOAuth更新結果は使わない |
| 共有ファイル | workspace・追加マウントは指定した権限で変更可能 |
| ホスト管理権限 | 管理socketとBuildKitの利用者はroot相当 |

[セキュリティ設計と制限](doc/security.md)
