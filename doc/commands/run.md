# `codex-dock run` — サンドボックスコンテナの起動

> **日本語** | [English](../en/commands/run.md)
>
> [← コマンドリファレンス一覧](../commands.md)

Codex CLI を Docker コンテナ内で実行します。Auth Proxy とネットワーク隔離が自動的に設定されます。

> **ネットワーク隔離**: 起動時に各ワーカー専用の Docker `Internal` ネットワーク（`dock-net-w-<name>`）を作成し、Auth Proxy をそこへ接続します。ワーカー間・ホスト・インターネットへの直接通信は Docker により遮断されます（`iptables`/`sudo` 不要、macOS / Windows でも同等）。

> **イメージの自動ビルド**: `--image`（または `--template`）で指定したイメージがローカルに存在しない場合、自動的にビルドしてからコンテナを起動します。テンプレートに対応するタグ（例: `codex-dock:pwn`）の場合はテンプレートからビルドされ、それ以外は `codex-dock build` と同じロジックでビルドされます。`~/.config/codex-dock/templates/` に置いたユーザーテンプレートも同様に自動ビルド対象です（[`codex-dock build` のテンプレート節](build.md#ユーザーテンプレート)を参照）。
>
> さらに、テンプレートの Dockerfile がディスク上のファイルとして存在する場合（ユーザーテンプレート、または `make install-templates` で配置した組み込みテンプレートのコピー）、そのファイルの更新日時とビルド済みイメージの作成日時を比較し、**ファイルの方が新しければ自動的に再ビルド**してから起動します。この挙動は `auto_build`（グローバル）/ `run.auto_build`（`run` のみ）の設定でオン・オフできます（デフォルトは有効）。詳細は [設定リファレンス](../configuration.md#auto_build) を参照してください。バイナリに埋め込まれたままの組み込みテンプレートには比較対象のファイルが無いため、この機能は働きません。

```bash
codex-dock run [OPTIONS]
```

---

## オプション一覧

| オプション | 省略形 | デフォルト | 説明 |
|---|---|---|---|
| `--image` | `-i` | `codex-dock:latest` | サンドボックスに使用する Docker イメージ |
| `--template` | `-T` | | サンドボックスイメージテンプレート（`plain`, `pwn` 等）。指定したテンプレートのタグに解決される。`--image` を明示指定した場合はそちらが優先。存在しなければ自動ビルドされる |
| `--pkg` | `-p` | | 追加パッケージ（繰り返し指定可）`apt:<pkg>`, `pip:<pkg>`, `npm:<pkg>` |
| `--pkg-file` | | | パッケージ定義ファイルのパス (`packages.dock`) |
| `--project` | `-d` | `.`（カレント） | `/workspace` にマウントするプロジェクトディレクトリ |
| `--worktree` | `-w` | `false` | git worktree を使ってコンテナを分離する |
| `--branch` | `-b` | | チェックアウトするブランチ名（`--worktree` が必要） |
| `--new-branch` | `-B` | `false` | 新規ブランチを作成する（`--worktree` と `--branch` が必要） |
| `--name` | `-n` | 自動生成 | コンテナ名（省略すると `codex-<形容詞>-<名詞>` 形式で自動生成） |
| `--agent` | | `""`（シェル） | 起動するエージェント（[詳細](#--agent--エージェントの選択)） |
| `--task` | `-t` | | エージェントに渡す初期タスクプロンプト |
| `--exec` | `-e` | | エージェント未指定時に、対話シェルの代わりに実行するコマンド（[詳細](#--exec--コマンドの実行)） |
| `--approval-mode` | | `suggest` | 承認モード（[詳細](#--approval-mode--承認モードの指定)） |
| `--full-auto` | | `false` | **非推奨**: `--approval-mode full-auto` を使用してください |
| `--model` | `-m` | | エージェントに渡すモデル名 |
| `--read-only` | | `false` | プロジェクトディレクトリを読み取り専用でマウント |
| `--mount` | | | 追加のホストディレクトリをマウント（繰り返し指定可）`<host-path>:<container-path>[:ro\|rw]`（[詳細](#--mount--任意ディレクトリの追加マウント)） |
| `--no-internet` | | `false` | 一般 egress を無効化（`HTTP(S)_PROXY` を注入せず、プロキシの API リバースルートのみ到達可能に） |
| `--keep` | | `false` | フォアグラウンド実行の終了時にコンテナと専用ネットワークを削除しない（既定では削除し、ネットワークの蓄積を防ぐ） |
| `--token-ttl` | | `3600` | Auth Proxy トークンの有効期限（秒） |
| `--agents-md` | | | `AGENTS.md` ファイルのパス |
| `--detach` | `-D` | `false` | バックグラウンドで実行（ログを表示しない） |
| `--parallel` | `-P` | `1` | 並列ワーカー数 |
| `--shell` | `-s` | `false` | `entrypoint.sh` をバイパスした素の（未認証の）シェルを起動（デバッグ用） |
| `--shell-type` | | `bash` | 対話シェルの種類（[詳細](#--shell-type--シェルの種類の指定)） |
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

## `--exec` — コマンドの実行

`--exec` フラグで、対話シェルの代わりにコンテナ内で1回だけ実行するコマンドを指定できます。
コンテナ内では `DOCK_EXEC_CMD` 環境変数として渡され、`entrypoint.sh` の「エージェント未指定」
分岐（認証設定済みシェルを起動する分岐）でのみ参照されます。

- `--agent` を指定した場合は**エージェントが常に優先され**、`--exec` は無視されます。
- `--shell`（`entrypoint.sh` を完全にバイパスする素のデバッグシェル）と併用するとエラーになります
  （`--shell` では `entrypoint.sh` 自体が実行されないため `--exec` が効きません）。
- コマンドは `--shell-type` で選んだシェル（既定は bash）の `-c` 経由で実行されます。

```bash
# 認証設定済みの環境でコマンドを1つ実行して終了
codex-dock run --exec "ls -la"

# バックグラウンドでコマンドを実行
codex-dock run --exec "npm test" --detach

# --agent を指定すると --exec は無視され、エージェントが起動する
codex-dock run --agent codex --exec "ls -la"   # → Codex CLI が起動する
```

---

## `--shell-type` — シェルの種類の指定

`--shell-type` で、サンドボックス内の対話シェルに使うバイナリを選べます。
対象は「エージェント未指定時の認証済みシェル」と「`--shell` デバッグモード」の両方です
（コンテナ内では `DOCK_SHELL` 環境変数として渡され、`entrypoint.sh` が参照します）。

| 値 | 動作 |
|---|---|
| `bash`（デフォルト） | 従来通り bash を起動 |
| `zsh` | zsh を起動。対象イメージに zsh がインストールされている必要がある |

組み込みの `plain` テンプレートには zsh がインストール済みで、`pwn` は `plain` を継承するため
（`FROM codex-dock:latest`）、どちらのテンプレートでも `--shell-type zsh` がそのまま使えます。
自作テンプレートで zsh を使う場合は、そのテンプレートの Dockerfile 側で zsh をインストールしてください。

> エージェント（`--agent codex`/`claude`）を Ctrl+Z で一時停止してシェルに戻る機構
> （`launch_interactive`）は、bash の `PROMPT_COMMAND` を使った実装のため、
> `--shell-type` の値に関わらず内部的には常に bash で動作します。`--shell-type` が
> 影響するのはエージェント未指定時の対話シェルと `--shell` デバッグモードのみです。

```bash
# zsh で対話シェルを起動
codex-dock run --shell-type zsh

# zsh の素のデバッグシェル（entrypoint.sh をバイパス）
codex-dock run --shell --shell-type zsh
```

設定ファイル（`~/.config/codex-dock/config.toml`）の `default_shell_type`（グローバル）/
`run.shell_type`（`run` のみ）でデフォルト値を変更できます。詳細は
[設定リファレンス](../configuration.md#default_shell_type) を参照してください。

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

## `--mount` — 任意ディレクトリの追加マウント

`--mount` フラグで、`/workspace`（`--project`）以外の任意のホストディレクトリを
コンテナへバインドマウントできます。繰り返し指定することで複数のディレクトリを
マウントできます。

```
--mount <host-path>:<container-path>[:ro|rw]
```

- `<host-path>`: ホスト側のディレクトリ（相対パスは実行時のカレントディレクトリから解決）
- `<container-path>`: コンテナ内の絶対パス。`/workspace` は `--project` 専用のため指定不可
- `:ro` / `:rw`: 読み取り専用 / 読み書き可能（省略時は `rw`）
- 同じ `<container-path>` を複数回指定するとエラーになる

```bash
# ホストの共有データディレクトリを読み取り専用でマウント
codex-dock run --mount /home/user/datasets:/data:ro

# 複数のディレクトリをマウント（読み書き・読み取り専用を混在）
codex-dock run --mount ./cache:/cache --mount /opt/models:/models:ro
```

> 追加マウントされたディレクトリはプロジェクトディレクトリ本体（`/workspace`）とは
> 独立して読み取り専用を指定できます。`--read-only` は `/workspace` のみに適用されます。

設定ファイル（`~/.config/codex-dock/config.toml`）の `[run]` セクションでも
`mounts` として既定値を指定できます（`--mount` を指定した場合はそちらが優先）。
詳細は [`doc/configuration.md`](../configuration.md#runmounts) を参照してください。

```toml
[run]
mounts = ["/home/user/datasets:/data:ro", "./cache:/cache"]
```

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

# テンプレートイメージを使用（未ビルドなら自動ビルド）
codex-dock run --image codex-dock:pwn

# --template でテンプレート名から直接指定（--image codex-dock:pwn と同じ）
codex-dock run --template pwn

# ~/.config/codex-dock/templates/ に置いた自作テンプレートも同様に使える
codex-dock run --template mytool

# カスタムモデルを指定
codex-dock run --model "o3"

# エージェントを使わずコマンドを1つ実行して終了
codex-dock run --exec "ls -la"

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
- [ネットワーク仕様](../network.md) — プロキシルータ + per-worker Internal ネットワークの構成
- [設定リファレンス](../configuration.md) — config.toml の設定項目
