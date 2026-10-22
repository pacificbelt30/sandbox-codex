# 御AgentIsolation（Oaiso）

**Codex CLI・Claude Codeを、ワーカーごとのKata VMで隔離して実行。**

| 項目 | 内容 |
|---|---|
| 正式名称 | 御AgentIsolation。「御」は丁寧さ・敬意を添える接頭辞 |
| 英字表記・読み | Oaiso／おあいそ |
| コマンド | `oaiso` |
| 実行ホスト | Linux x86_64・KVM・containerd・Kata runtime-rs・Dragonball |
| イメージ作成 | BuildKit |
| 実行時に不要 | Docker Engine・Docker CLI・QEMU |

[日本語ドキュメント](doc/index.md) · [English documentation](doc/en/index.md)

## Installation — インストール

- 対象：**Ubuntu 26.04・x86_64・systemd・KVM・cgroup v2**。
- 実行者：`sudo` が使える通常ユーザー。管理操作はスクリプトが昇格。
- ビルド要件：Go 1.24.7以上・Python 3.11以上。
- ほかのLinux：[手動配置](doc/kata.md)。WSL／VM：上記要件とnested virtualizationが必要。

**1. 依存ツール・ソースを取得し、インストール**

```bash
sudo apt-get update
sudo apt-get install -y git make python3 golang-go
go version

git clone https://github.com/pacificbelt30/sandbox-codex.git
cd sandbox-codex

./scripts/install.sh          # 配置計画を確認
./scripts/install.sh --apply  # CLI・Kata・containerd・BuildKit・サービスを配置
```

**2. ログアウト・再ログイン後、確認**

```bash
oaiso --version
oaiso runtime-check
```

**3. 認証を選び、初回起動**

- [クイックスタートの「認証方式を選ぶ」](doc/getting-started.md#2-認証方式を選ぶ)へ進む。
- 詳細・配置先・更新：[Installation](doc/installation.md)。
- `make install` はCLI・補助ツール・補完の配置のみ。実行ホストの準備には上の手順を使用。

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
| 1 | 管理者 | [インストール](doc/installation.md)。Ubuntu 26.04対応ホストでは `./scripts/install.sh` で計画、`--apply` で適用 |
| 2 | 利用者 | [認証とイメージを準備](doc/getting-started.md)。ホストと分ける場合は[独立プロファイル](doc/commands/auth.md#独立プロファイル)を選択 |
| 3 | 利用者 | プロジェクトでワーカーを起動 |

**先に認証を設定（いずれか1つ）**

| 認証方式 | 準備するもの |
|---|---|
| 独立プロファイル | [登録・選択・管理シークレット設定](doc/guides/authentication.md)。ホストCLIの認証と分ける |
| 従来のホスト連携 | [エージェント別の認証ソース](doc/getting-started.md#2-認証方式を選ぶ)を準備 |

- 以下はどちらか1つ。`/path/to/your-project` は実際の作業プロジェクトへ置換。
- Git・npm・pipなどを使う場合は、先に[一般通信の宛先](doc/guides/networking.md)も許可。

**Codex：製品ソースのルートで準備 → 作業先で起動**

```bash
oaiso init --agent codex
cd /path/to/your-project
oaiso codex

# Codex CLI の引数もそのまま渡せる
oaiso codex "このリポジトリを調べて"

```

**Claude Code：製品ソースのルートで準備 → 作業先で起動**

```bash
oaiso init --agent claude
cd /path/to/your-project
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
oaiso stop WORKER
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
| はじめに：インストールから初回起動 | [Installation](doc/installation.md) → [クイックスタート](doc/getting-started.md) |
| 利用ガイド：エージェント・作業環境・認証・通信 | [利用ガイド](doc/guides/index.md) |
| リファレンス：CLI・設定・組み込みAPI | [リファレンス](doc/reference/index.md) |
| 管理・運用：日常操作・更新・障害対応 | [日常操作](doc/operations.md)・[ホスト更新](doc/setup.md)・[トラブルシューティング](doc/troubleshooting.md) |
| 開発：構成・上流追従・検証 | [アーキテクチャ](doc/architecture.md)・[上流互換性](doc/upstream-compatibility.md)・[テスト](doc/testing.md) |

[全体の章立て](doc/index.md#章から探す) · [English chapters](doc/en/index.md#browse-chapters)

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
- 章の分類・追加先：[ドキュメントの構成と編集](doc/development/documentation.md)。

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
