# コマンドリファレンス

> **日本語** | [English](en/commands.md)

## グローバルオプション

すべてのコマンドで使用できるオプションです。

| オプション | 省略形 | デフォルト | 説明 |
|---|---|---|---|
| `--verbose` | `-v` | `false` | 詳細なログを出力する |
| `--debug` | | `false` | デバッグログを出力する |
| `--config` | | `~/.config/codex-dock/config.toml` | 設定ファイルのパス |

---

## `codex-dock run` — サンドボックスコンテナの起動

Codex CLI を Docker コンテナ内で実行します。Auth Proxy とネットワーク隔離が自動的に設定されます。

> **イメージの自動ビルド**: `--image` で指定したイメージがローカルに存在しない場合、`codex-dock build` と同じロジックで自動的にビルドしてからコンテナを起動します。

```bash
codex-dock run [OPTIONS]
```

### オプション

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
| `--approval-mode` | | `suggest` | Codex CLI の承認モード（詳細は下記） |
| `--full-auto` | | `false` | **非推奨**: `--approval-mode full-auto` を使用してください |
| `--model` | `-m` | | Codex に渡すモデル名 |
| `--read-only` | | `false` | プロジェクトディレクトリを読み取り専用でマウント |
| `--no-internet` | | `false` | コンテナ内のインターネットアクセスを無効化 |
| `--token-ttl` | | `3600` | Auth Proxy トークンの有効期限（秒） |
| `--agents-md` | | | `AGENTS.md` ファイルのパス |
| `--detach` | `-D` | `false` | バックグラウンドで実行（ログを表示しない） |
| `--parallel` | `-P` | `1` | 並列ワーカー数 |
| `--user` | | `""` | コンテナ内の実行ユーザ（詳細は下記） |

### `--approval-mode` — Codex 承認モードの指定

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

### `--user` — コンテナ実行ユーザの指定

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

### 使用例

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

### パッケージ記述形式

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

### 並列ワーカー

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

## `codex-dock proxy` — Auth Proxy のビルド・起動

Auth Proxy コンテナのビルドと起動をサブコマンドで実行します。

```bash
codex-dock proxy build [OPTIONS]
codex-dock proxy run [OPTIONS]
```

### `codex-dock proxy build` オプション

| オプション | 省略形 | デフォルト | 説明 |
|---|---|---|---|
| `--tag` | `-t` | `codex-dock-auth-proxy:latest` | ビルドするイメージタグ |
| `--dockerfile` | `-f` | 自動検出 | Auth Proxy Dockerfile のパス |

`--dockerfile` 未指定時は、`docker/auth-proxy.Dockerfile` → `auth-proxy.Dockerfile` の順で探索し、見つからない場合は `~/.config/codex-dock/auth-proxy.Dockerfile` を自動生成して使用します。

### `codex-dock proxy run` オプション

| オプション | 省略形 | デフォルト | 説明 |
|---|---|---|---|
| `--image` | `-i` | `codex-dock-auth-proxy:latest` | 起動するイメージ |
| `--name` | `-n` | `codex-auth-proxy` | コンテナ名 |
| `--network` | | `dock-net` | 接続する Docker ネットワーク |
| `--admin-secret` | | `""` | `/admin/*` エンドポイント用シークレット |

`proxy run` は `dock-net` が存在しない場合に自動作成し、既存の同名コンテナがあれば置き換えて起動します。

---

## `codex-dock ps` — ワーカー一覧

実行中のコンテナを一覧表示します。

```bash
codex-dock ps [OPTIONS]
```

| オプション | 省略形 | デフォルト | 説明 |
|---|---|---|---|
| `--all` | `-a` | `false` | 停止済みのコンテナも含めて表示 |

**出力例：**

```
NAME                   STATUS    UPTIME    BRANCH         TASK
codex-brave-atlas      running   5m23s     feature-auth   ユニットテスト作成
codex-calm-beacon      running   2m10s     main           (interactive)
```

---

## `codex-dock stop` — コンテナの停止

実行中のコンテナを停止します。

```bash
codex-dock stop [NAME|ID...] [OPTIONS]
```

| オプション | 省略形 | デフォルト | 説明 |
|---|---|---|---|
| `--all` | `-a` | `false` | 実行中の全コンテナを停止 |
| `--timeout` | | `10` | 強制停止までの待機時間（秒） |

**使用例：**

```bash
# 特定のコンテナを停止
codex-dock stop codex-brave-atlas

# 複数のコンテナを停止
codex-dock stop codex-brave-atlas codex-calm-beacon

# 全コンテナを停止
codex-dock stop --all
```

---

## `codex-dock rm` — コンテナの削除

停止済みのコンテナを削除します。

```bash
codex-dock rm [NAME|ID...] [OPTIONS]
```

| オプション | 省略形 | デフォルト | 説明 |
|---|---|---|---|
| `--force` | `-f` | `false` | 実行中のコンテナも強制削除 |

**使用例：**

```bash
# 停止済みコンテナを削除
codex-dock rm codex-brave-atlas

# 実行中のコンテナを強制削除
codex-dock rm --force codex-brave-atlas
```

---

## `codex-dock logs` — ログの表示

コンテナのログを表示します。

```bash
codex-dock logs NAME|ID [OPTIONS]
```

| オプション | 省略形 | デフォルト | 説明 |
|---|---|---|---|
| `--tail` | `-n` | `100` | 末尾から表示する行数 |
| `--follow` | `-f` | `false` | ログをリアルタイムで追跡する |

**使用例：**

```bash
# 直近 100 行を表示
codex-dock logs codex-brave-atlas

# リアルタイムでログを追跡
codex-dock logs codex-brave-atlas --follow

# 直近 50 行を表示
codex-dock logs codex-brave-atlas --tail 50
```

---

## `codex-dock auth` — 認証管理

API キーや OAuth の認証情報を管理します。

### `auth show` — 認証状態の確認

```bash
codex-dock auth show
```

現在の認証ソースを表示します（実際のキーやトークンは表示されません）。

**出力例（API キーの場合）：**
```
Auth source: OPENAI_API_KEY env
Configured:  yes
```

**出力例（OAuth の場合）：**
```
Auth source: ~/.codex/auth.json (OAuth/ChatGPT subscription)
Configured:  yes
```

**出力例（未設定の場合）：**
```
Auth source: none
Configured:  no
```

### `auth set` — API キーの保存

```bash
export OPENAI_API_KEY=sk-...
codex-dock auth set
```

`OPENAI_API_KEY` 環境変数の値を `~/.config/codex-dock/apikey` に保存します。
パーミッションは `0600` で保護されます。

### `auth rotate` — トークンのローテーション

```bash
codex-dock auth rotate
```

現在発行中の全トークンを無効化します。

---

## `codex-dock network` — ネットワーク管理

`dock-net` Docker ネットワークを管理します。

### `network create` — ネットワーク作成

```bash
codex-dock network create [--no-internet]
```

| オプション | 説明 |
|---|---|
| `--no-internet` | IP Masquerade を無効化してインターネットを遮断 |

> `codex-dock run` 実行時に自動的に作成されます。

### `network rm` — ネットワーク削除

```bash
codex-dock network rm
```

> 実行中のコンテナがある場合は先に停止してください。

### `network status` — ネットワーク状態確認

```bash
codex-dock network status
```

**出力例：**
```
dock-net status:
  ID:            a1b2c3d4e5f6789012345678
  Driver:        bridge
  Subnet:        192.168.200.0/24
  ICC:           disabled
  IP Masquerade: enabled
```

---

## `codex-dock build` — イメージのビルド

サンドボックス用の Docker イメージをビルドします。

```bash
codex-dock build [OPTIONS]
```

| オプション | 省略形 | デフォルト | 説明 |
|---|---|---|---|
| `--tag` | `-t` | `codex-dock:latest` | イメージのタグ |
| `--dockerfile` | `-f` | （自動検出） | Dockerfile のパス |

### Dockerfile の検索順序

`-f` を省略した場合、以下の順序で Dockerfile を自動検出します：

1. カレントディレクトリの `Dockerfile`
2. カレントディレクトリの `docker/Dockerfile`
3. `~/.config/codex-dock/Dockerfile`（存在しない場合は組み込みデフォルトを自動書き出し）

> **補足**: `~/.config/codex-dock/` へのフォールバック時は `entrypoint.sh` も同ディレクトリに書き出されます。
> ユーザーが同ファイルを編集済みの場合は上書きされません。

**使用例：**

```bash
# デフォルト設定でビルド（Dockerfile を自動検出）
codex-dock build

# カスタムタグでビルド
codex-dock build --tag my-codex:v1

# Dockerfile を明示指定
codex-dock build -f /path/to/Dockerfile

# カスタムイメージをビルドして run で使用
codex-dock build -f ./custom/Dockerfile --tag my-codex:v2
codex-dock run --image my-codex:v2
```

---

## `codex-dock ui` — TUI ダッシュボード

全ワーカーをリアルタイムで監視・管理するターミナル UI を起動します。

```bash
codex-dock ui
```

### 画面レイアウト

```
┌──────────────────────────────────────────────────────────────┐
│ codex-dock  [実行中: 2 / 合計: 4]                            │
├──────────────────────────────────────────────────────────────┤
│  NAME              BRANCH      STATUS      UPTIME  TASK       │
│  codex-brave-atl   feature-1   running     5m23s   テスト作成 │
│▶ codex-calm-bea    main        running     2m10s   (interactive)│
│  codex-old-comet   feature-2   exited      -       完了       │
├──────────────────────────────────────────────────────────────┤
│ [↑↓] 選択  [Enter] ログ  [S] 停止  [D] 削除  [A] 全停止  [Q] 終了 │
└──────────────────────────────────────────────────────────────┘
```

### キーバインド

| キー | 動作 | 実装状況 |
|---|---|---|
| `↑` / `↓` | コンテナを選択 | ✅ |
| `Enter` | ログビューを表示 | ⚠️ スタブ表示 |
| `S` | 選択したコンテナを停止 | ✅ |
| `D` | 選択したコンテナを削除 | ✅ |
| `A` | 全コンテナを停止 | ✅ |
| `R` | コンテナを起動 | ❌ 未実装 |
| `Q` | UI を終了 | ✅ |

> **更新間隔**: コンテナ一覧は 2 秒ごとに自動更新されます。
