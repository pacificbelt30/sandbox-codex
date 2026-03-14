# クイックスタート

> **日本語** | [English](en/getting-started.md)

codex-dock のインストールから最初のコンテナ起動までの手順を説明します。

---

## 前提条件

| 要件 | バージョン | 確認方法 |
|---|---|---|
| Go | 1.24 以上 | `go version` |
| Docker Engine | 20.10 以上 | `docker version` |
| git | 任意 | worktree 機能を使う場合に必要 |

---

## インストール

### Makefile を使う（推奨）

Makefile を使うと、バイナリのビルド・インストール・デフォルト設定ファイルの配置・Docker イメージの作成をまとめて行えます。

```bash
# リポジトリをクローン
git clone https://github.com/pacificbelt30/codex-dock.git
cd codex-dock

# バイナリをビルドし、codex-dock:latest イメージを作成
make all

# バイナリを /usr/local/bin に配置し、デフォルト設定を ~/.config/codex-dock/config.toml に配置
sudo make install-all
```

上記 3 ステップで以下がすべて完了します：

| 完了する内容 | 場所 |
|---|---|
| バイナリ配置 | `/usr/local/bin/codex-dock` |
| デフォルト設定 | `~/.config/codex-dock/config.toml` |
| サンドボックスイメージ | `codex-dock:latest`（Docker） |
| Auth Proxy イメージ | `codex-dock-proxy:latest`（Docker） |

> **注意**: `sudo make install-all` は `$SUDO_USER` を参照して実行ユーザーのホームディレクトリを特定するため、config は `/root/.config/` ではなく `~/config/codex-dock/` に配置されます。

インストール先を変更したい場合：

```bash
sudo make install PREFIX=/opt/codex-dock
```

ステップごとに個別実行することもできます：

```bash
make build          # バイナリのみビルド
make docker         # Docker イメージのみビルド
make install-config # 設定ファイルのみ配置（既存ならスキップ）
make uninstall      # バイナリをアンインストール
make clean          # ビルド成果物を削除
```

### ソースからビルド（手動）

```bash
# リポジトリをクローン
git clone https://github.com/pacificbelt30/codex-dock.git
cd codex-dock

# バイナリをビルド
go build -o codex-dock .

# パスに追加（任意）
sudo mv codex-dock /usr/local/bin/
```

---

## Step 0: Auth Proxy を起動する（必須）

`codex-dock run` は外部 Auth Proxy（既定: `http://127.0.0.1:18080`）へ接続します。先に Auth Proxy を起動してください。

### Docker で起動（推奨）

```bash
# 1) dock-net を作成（初回のみ）
codex-dock network create

# 2) Auth Proxy イメージをビルド
codex-dock proxy build

# 3) Auth Proxy コンテナを起動（認証情報は自動検出）
codex-dock proxy run

# 4) 推奨: firewall を設定（Linux + root）
sudo codex-dock firewall create --proxy-container-url http://codex-auth-proxy:18080
```


> **推奨**: `network create` と `proxy run` の後に firewall を設定してください。`codex-dock run` の通信制御が明確になります。詳細は [firewall 仕様・運用ガイド](firewall.md) を参照してください。

`codex-dock proxy run` は以下の認証情報を自動的にコンテナへバインドします：

| 認証方式 | ホスト側のソース | コンテナへの渡し方 |
|---|---|---|
| API キー（環境変数） | `OPENAI_API_KEY` | `-e OPENAI_API_KEY=<値>` |
| API キー（保存済み） | `~/.config/codex-dock/apikey` | bind-mount（読み取り専用） |
| OAuth / ChatGPT | `~/.codex/auth.json` | bind-mount（読み取り専用） |

存在するすべてのソースが同時にバインドされます。プロキシ起動時の優先順位は `OPENAI_API_KEY` 環境変数 → 保存済みキーファイル → OAuth の順です。

#### コンテナの停止・削除

```bash
codex-dock proxy stop   # コンテナを停止
codex-dock proxy rm     # コンテナを削除
```

### ローカルプロセスとして起動

```bash
codex-dock proxy serve --listen 0.0.0.0:18080
```

この場合、ワーカーコンテナから到達できる URL を `--proxy-container-url` で指定してください。

> `connecting external auth proxy: connecting remote auth proxy ... connect: connection refused` が出る場合は、Auth Proxy が未起動です。上記のどちらかの方法で起動後に `codex-dock run` を再実行してください。

---

## Step 1: サンドボックスイメージのビルド

コンテナ内で Codex CLI を実行するためのイメージをビルドします。

```bash
codex-dock build
```

**内部で行われること：**
- Dockerfile を自動検出（カレントディレクトリ → `~/.config/codex-dock/`）
- Node.js 22 ベース + Codex CLI (`@openai/codex`) をインストール
- 非 root ユーザー `codex` (uid:1001) を作成
- デフォルトタグ: `codex-dock:latest`

> **省略可能**: `codex-dock run` 実行時にイメージが存在しない場合は自動的にビルドされます。
> 初回は `codex-dock run` だけで完結します。

カスタム Dockerfile を使いたい場合は `-f` で指定できます：

```bash
codex-dock build -f ./my/Dockerfile --tag my-codex:v1
```

---

## Step 2: 認証の設定

### API キーを使う場合

```bash
export OPENAI_API_KEY=sk-...
codex-dock auth set
```

または、実行のたびに環境変数を設定するだけでも動作します：

```bash
export OPENAI_API_KEY=sk-...
codex-dock run
```

### ChatGPT サブスクリプション（OAuth）を使う場合

まず通常の Codex CLI でログインします：

```bash
codex login
```

`~/.codex/auth.json` が生成されれば、codex-dock が自動的に検出します。

```bash
# 認証状態を確認
codex-dock auth show
# Auth source: ~/.codex/auth.json (OAuth/ChatGPT subscription)
```

---

## Step 3: 最初のコンテナを起動する

```bash
# カレントディレクトリを /workspace にマウントして Codex を起動
codex-dock run --user current --approval-mode full-auto
```

`--user current` を付けると、ホスト側の現在ユーザーと同じ uid:gid でコンテナを起動できます。
生成されるファイルの所有者がずれにくいため、通常はこの指定を推奨します。

`--approval-mode full-auto` は現在の推奨設定です。旧 `--full-auto` フラグは非推奨です。

**実行時に行われること：**

```
1. dock-net の作成確認（なければ作成）
       │
       ▼
2. 外部 Auth Proxy へ接続確認
   (既定: http://127.0.0.1:18080)
       │
       ▼
3. 短命トークンを発行
       │
       ▼
4. コンテナを作成・起動
   - Network: dock-net
   - Mount:   ./  →  /workspace
   - env:     CODEX_TOKEN=cdx-xxxx
       │
       ▼
5. コンテナ内で entrypoint.sh が実行
   → Auth Proxy から認証情報を取得
   → Codex CLI を起動
```

---

## よく使うパターン

### タスクを指定して自動実行

```bash
codex-dock run \
  --user current \
  --approval-mode full-auto \
  --task "src/auth.go のユニットテストを追加してください" \
  --detach
```

完了後にログを確認：

```bash
codex-dock ps --all
codex-dock logs codex-brave-atlas --tail 50
```

### git worktree で安全にブランチ作業

元のリポジトリを汚さずに新しいブランチで作業できます。

```bash
codex-dock run \
  --user current \
  --approval-mode full-auto \
  --worktree \
  --branch feature-auth \
  --new-branch \
  --task "認証モジュールをリファクタリングして"
```

コンテナが終了すると worktree は自動的に削除されます。

### 並列ワーカーで複数タスクを同時実行

```bash
# 3 つのブランチで並列実行
codex-dock run --user current --approval-mode full-auto --parallel 3 --worktree --detach
```

TUI で全ワーカーを監視：

```bash
codex-dock ui
```

### パッケージをインストールして実行

```bash
codex-dock run --pkg "apt:libssl-dev" --pkg "pip:cryptography"
```

または `packages.dock` ファイルで管理：

```bash
# packages.dock を作成
cat > packages.dock << 'EOF'
apt:libssl-dev
pip:cryptography
pip:pytest
npm:typescript
EOF

# 自動検出してインストール
codex-dock run
```

---

## ネットワークの確認

```bash
# dock-net の状態を確認
codex-dock network status

# 実行中のワーカーを確認
codex-dock ps
```

---

## コンテナの後片付け

```bash
# 全コンテナを停止
codex-dock stop --all

# 停止済みコンテナを削除
codex-dock rm <コンテナ名>

# dock-net を削除（必要な場合）
codex-dock network rm
```

---

## トラブルシューティング

### コンテナが起動しない

```bash
# 詳細ログで実行
codex-dock run --verbose --debug
```

よくある原因：
- Docker が起動していない → `docker ps` で確認
- イメージのビルドに失敗した → `codex-dock build --verbose` で確認
- API キーが未設定 → `codex-dock auth show` で確認

### 認証エラー

```bash
# 認証状態を確認
codex-dock auth show

# API キーを再設定
export OPENAI_API_KEY=sk-...
codex-dock auth set
```

### ネットワークエラー

```bash
# dock-net を作り直す
codex-dock network rm
codex-dock network create
```

### ログを確認する

```bash
# コンテナのログをリアルタイム確認
codex-dock logs <コンテナ名> --follow

# 全ワーカーの状態確認
codex-dock ps --all
```

---

## 次のステップ

- [アーキテクチャ概要](architecture.md) — システムの全体像を理解する
- [Auth Proxy 技術仕様](auth-proxy.md) — 認証の仕組みを詳しく学ぶ
- [ネットワーク仕様](network.md) — ネットワーク隔離の詳細
- [コマンドリファレンス](commands.md) — 全コマンドとオプションの一覧
- [設定リファレンス](configuration.md) — config.toml の設定項目
