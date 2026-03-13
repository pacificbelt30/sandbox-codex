# `codex-dock run` — サンドボックスコンテナの起動

> **日本語** | [English](../../en/commands/run.md)
>
> [← コマンドリファレンス一覧](../commands.md)

Codex CLI を Docker コンテナ内で実行します。Auth Proxy とネットワーク隔離が自動的に設定されます。

> **イメージの自動ビルド**: `--image` で指定したイメージがローカルに存在しない場合、`codex-dock build` と同じロジックで自動的にビルドしてからコンテナを起動します。

```bash
codex-dock run [OPTIONS]
```

---

## オプション一覧

| オプション | 省略形 | デフォルト | 説明 |
|---|---|---|---|
| `--image` | `-i` | `codex-dock:latest` | サンドボックスに使用する Docker イメージ |
| `--pkg` | `-p` | | 追加パッケージ（繰り返し指定可）`apt:<pkg>`, `pip:<pkg>`, `npm:<pkg>` |
| `--pkg-file` | | | パッケージ定義ファイルのパス (`packages.dock`) |
| `--project` | `-d` | `.`（カレント） | `/workspace` にマウントするプロジェクトディレクトリ |
| `--worktree` | `-w` | `false` | git worktree を使ってコンテナを分離する |
| `--branch` | `-b` | | チェックアウトするブランチ名（`--worktree` が必要） |
| `--new-branch` | `-B` | `false` | 新規ブランチを作成する（`--worktree` と `--branch` が必要） |
| `--name` | `-n` | 自動生成 | コンテナ名（省略すると `codex-<形容詞>-<名詞>` 形式で自動生成） |
| `--task` | `-t` | | Codex に渡す初期タスクプロンプト |
| `--approval-mode` | | `suggest` | Codex CLI の承認モード（[詳細](#--approval-mode--codex-承認モードの指定)） |
| `--full-auto` | | `false` | **非推奨**: `--approval-mode full-auto` を使用してください |
| `--model` | `-m` | | Codex に渡すモデル名 |
| `--read-only` | | `false` | プロジェクトディレクトリを読み取り専用でマウント |
| `--no-internet` | | `false` | コンテナ内のインターネットアクセスを無効化 |
| `--token-ttl` | | `3600` | Auth Proxy トークンの有効期限（秒） |
| `--agents-md` | | | `AGENTS.md` ファイルのパス |
| `--detach` | `-D` | `false` | バックグラウンドで実行（ログを表示しない） |
| `--parallel` | `-P` | `1` | 並列ワーカー数 |
| `--user` | | `""` | コンテナ内の実行ユーザ（[詳細](#--user--コンテナ実行ユーザの指定)） |

---

## `--approval-mode` — Codex 承認モードの指定

`--approval-mode` フラグで Codex CLI がアクションを実行する際の承認動作を制御します。
Docker コンテナによるサンドボックス隔離を前提に設計されています。

| 値 | Codex CLI フラグ | 動作 |
|---|---|---|
| `suggest`（デフォルト） | なし | すべてのアクションで承認を求める（最も安全） |
| `auto-edit` | `--ask-for-approval unless-allow-listed` | ファイル編集は自動適用、コマンド実行は承認を求める |
| `full-auto` | `--ask-for-approval never` | 承認を一切求めない |
| `danger` | `--dangerously-bypass-approvals-and-sandbox` | すべての承認・サンドボックス制限をバイパスする |

> **`danger` モードについて**: Codex CLI 組み込みのサンドボックスを無効化しますが、
> Docker コンテナ自体が隔離境界を提供します（`--cap-drop ALL`、専用ネットワーク、
> pids-limit など）。Docker を使用しているため、ホスト環境への影響はありません。

```bash
# デフォルト（対話的に承認を求める）
codex-dock run --task "テストを書いて"

# ファイル編集は自動、コマンドは確認
codex-dock run --approval-mode auto-edit --task "リファクタリング"

# 完全自動実行（承認なし）
codex-dock run --approval-mode full-auto --task "バグ修正"

# Docker 隔離を活用して全制限をバイパス
codex-dock run --approval-mode danger --task "ビルドスクリプトを実行"
```

---

## `--user` — コンテナ実行ユーザの指定

`--user` フラグを使うと、コンテナ内のプロセスを任意の uid:gid で起動できます。
ホストのファイルシステムとの権限整合性を確保するために使用します。

| 値 | 動作 |
|---|---|
| `""` (省略) | イメージのデフォルトユーザ（`codex` uid:1001） |
| `current` | `codex-dock` コマンドを実行しているユーザの uid:gid を自動取得 |
| `dir` | `--project` で指定したディレクトリ所有者の uid:gid を自動取得 |
| `uid` または `uid:gid` | 明示的に指定（例: `1000`, `1000:1000`） |

> **注意**: カスタムユーザを指定した場合、そのユーザがコンテナの `/etc/passwd` に存在しない
> ことがあります。`codex-dock` は `HOME=/tmp` を自動的に注入するため、認証ファイルや
> Codex CLI の設定は `/tmp` 以下に書き込まれます。コンテナ終了時に自動的に破棄されます。

---

## パッケージ記述形式

`--pkg` または `packages.dock` ファイルで使用できるパッケージ形式：

```
apt:libssl-dev          # apt でインストール
pip:requests            # pip でインストール
npm:lodash              # npm でインストール
libssl-dev              # プレフィックスなし → apt 扱い（デフォルト）
```

`packages.dock` ファイルの例：

```
# コメントは # で始める
apt:libssl-dev
apt:postgresql-client
pip:requests
pip:numpy
npm:typescript
```

---

## 並列ワーカー

`--parallel N` を指定すると、N 個のコンテナが同時に起動します。

```bash
codex-dock run --parallel 3 --worktree --branch myfeature
```

自動的に以下のブランチが作成されます：
- `myfeature-1`（worker 1）
- `myfeature-2`（worker 2）
- `myfeature-3`（worker 3）

`--branch` を指定しない場合は `worker-1`, `worker-2`, `worker-3` が使用されます。

---

## 使用例

```bash
# 基本的な実行（カレントディレクトリをマウント）
codex-dock run

# タスクを指定して完全自動実行
codex-dock run --task "ユニットテストを書いて" --approval-mode full-auto

# git worktree を使って feature ブランチで作業
codex-dock run --worktree --branch feature-auth --new-branch

# 並列ワーカー 3 つを起動（各ワーカーに別ブランチ）
codex-dock run --parallel 3 --worktree

# 追加パッケージをインストールして実行
codex-dock run --pkg "apt:libssl-dev" --pkg "pip:requests" --pkg "npm:lodash"

# packages.dock ファイルを使用
codex-dock run --pkg-file ./packages.dock

# バックグラウンドで完全自動実行
codex-dock run --task "リファクタリング" --approval-mode full-auto --detach

# 読み取り専用・インターネットなしでセキュアに実行
codex-dock run --read-only --no-internet --task "コードレビュー"

# 特定の Docker イメージを使用
codex-dock run --image my-custom-codex:v2

# カスタムモデルを指定
codex-dock run --model "o3"

# コマンド実行ユーザと同じ uid:gid でコンテナを起動（ファイル権限の整合性確保）
codex-dock run --user current

# プロジェクトディレクトリの所有者と同じ uid:gid でコンテナを起動
codex-dock run --user dir --project /srv/myapp

# uid:gid を明示指定
codex-dock run --user 1000:1000
```

---

## 関連ドキュメント

- [クイックスタート](../getting-started.md) — 初回実行の手順
- [アーキテクチャ概要](../architecture.md) — 起動シーケンスの詳細
- [Auth Proxy 技術仕様](../auth-proxy.md) — 認証の仕組み
- [ネットワーク仕様](../network.md) — dock-net の構成
- [設定リファレンス](../configuration.md) — config.toml の設定項目
