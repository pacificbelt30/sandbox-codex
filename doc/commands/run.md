# `codex-dock run` — サンドボックスコンテナの起動

> **日本語** | [English](../../en/commands/run.md)
>
> [← コマンドリファレンス一覧](../commands.md)

Codex CLI を Docker コンテナ内で実行します。Auth Proxy とネットワーク隔離が自動的に設定されます。

> **Linux の注意**: 起動時に `dock-net` 用の `iptables` ルール適用を試みます。
> root 権限がない場合は Warning を表示して起動を継続します。
> firewall を明示的に適用したい場合は `codex-dock firewall create` を使用してください。

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
| `--agent` | | `""`（シェル） | 起動するエージェント（[詳細](#--agent--エージェントの選択)） |
| `--task` | `-t` | | エージェントに渡す初期タスクプロンプト |
| `--approval-mode` | | `suggest` | 承認モード（[詳細](#--approval-mode--承認モードの指定)） |
| `--full-auto` | | `false` | **非推奨**: `--approval-mode full-auto` を使用してください |
| `--model` | `-m` | | エージェントに渡すモデル名 |
| `--read-only` | | `false` | プロジェクトディレクトリを読み取り専用でマウント |
| `--no-internet` | | `false` | コンテナ内のインターネットアクセスを無効化 |
| `--no-firewall` | | `false` | codex-dock の dock-net iptables ルール適用をスキップ（ホスト側ファイアウォールに委ねる） |
| `--allow-host` | | | dock-net ファイアウォールで追加許可する宛先 `IP:PORT`（繰り返し指定可） |
| `--block-host` | | | dock-net ファイアウォールで追加遮断する宛先 `CIDR`/`IP`/`IP:PORT`（IPv4・繰り返し指定可） |
| `--sudo` | | `false` | root でないとき dock-net の iptables 適用のみ `sudo` 経由で実行（対話端末では一度だけパスワード要求、非対話環境では NOPASSWD/キャッシュ資格情報を利用） |
| `--token-ttl` | | `3600` | Auth Proxy トークンの有効期限（秒） |
| `--agents-md` | | | `AGENTS.md` ファイルのパス |
| `--detach` | `-D` | `false` | バックグラウンドで実行（ログを表示しない） |
| `--parallel` | `-P` | `1` | 並列ワーカー数 |
| `--user` | | `""` | コンテナ内の実行ユーザ（[詳細](#--user--コンテナ実行ユーザの指定)） |

---

## `--agent` — エージェントの選択

`--agent` フラグでサンドボックス内に起動する AI エージェントを選びます。
コンテナ内では `DOCK_AGENT` 環境変数として渡され、`entrypoint.sh` が分岐します。

| 値 | 動作 | 注入される認証情報 |
|---|---|---|
| _（省略）_ | 認証設定済みの対話シェルを起動（`codex` / `claude` 両方が PATH 上にある） | Codex と Anthropic の両方（利用可能なもの） |
| `codex` | OpenAI Codex CLI を起動 | `CODEX_AUTH_PROXY_URL`, `CODEX_TOKEN`, `OPENAI_BASE_URL` ほか |
| `claude` | Anthropic Claude Code を起動 | `ANTHROPIC_BASE_URL`(`…/anthropic`), `ANTHROPIC_API_KEY`（プレースホルダ） |

- `--agent claude` は Auth Proxy が Anthropic 認証情報を持たない場合、起動前にエラーで停止します
  （Proxy の `/admin/mode` を参照）。
- `--shell` は `entrypoint.sh` を完全にバイパスして**認証なし**の素のシェルを起動します（デバッグ用）。
  認証済みシェルが欲しい場合は `--agent` を省略してください。

```bash
codex-dock run --agent codex  --approval-mode full-auto
codex-dock run --agent claude --approval-mode full-auto --task "Refactor the auth module"
codex-dock run                # 認証済みシェル（codex / claude 両方利用可）
```

---

## `--approval-mode` — 承認モードの指定

`--approval-mode` フラグでエージェントがアクションを実行する際の承認動作を制御します。
Docker コンテナによるサンドボックス隔離を前提に設計されています。値は両エージェントに対応する
フラグへマッピングされます。

| 値 | Codex CLI フラグ | Claude Code フラグ | 動作 |
|---|---|---|---|
| `suggest`（デフォルト） | なし | なし | すべてのアクションで承認を求める（最も安全） |
| `auto-edit` | `--ask-for-approval unless-allow-listed` | `--permission-mode acceptEdits` | ファイル編集は自動適用、コマンド実行は承認を求める |
| `full-auto` | `--ask-for-approval never` | `--dangerously-skip-permissions` | 承認を一切求めない |
| `danger` | `--dangerously-bypass-approvals-and-sandbox` | `--dangerously-skip-permissions` | すべての承認・サンドボックス制限をバイパスする |

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
| `current` (省略時のデフォルト) | `codex-dock` コマンドを実行しているユーザの uid:gid を自動取得 |
| `codex` | 従来のデフォルト挙動。コンテナ内 `codex` ユーザ（`1001:1001`）で実行 |
| `""` | イメージのデフォルトユーザを使用 |
| `dir` | `--project` で指定したディレクトリ所有者の uid:gid を自動取得 |
| `uid` または `uid:gid` | 明示的に指定（例: `1000`, `1000:1000`） |

> **注意**: カスタムユーザを指定した場合、そのユーザがコンテナの `/etc/passwd` に存在しない
> ことがあります。`codex-dock` は `HOME=/var/tmp/codex-home` を自動注入します。認証ファイルや
> Codex CLI 設定はその `HOME` 配下に作成され、`/workspace` 配下に補助ディレクトリは作成しません。

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

# デフォルトで、コマンド実行ユーザと同じ uid:gid でコンテナを起動
codex-dock run

# 従来どおり codex(1001:1001) ユーザで実行
codex-dock run --user codex

# 明示的にコマンド実行ユーザを指定
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
