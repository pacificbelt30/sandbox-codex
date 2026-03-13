# codex-dock

**AI Sandbox Container Manager** — Runs [Codex CLI](https://github.com/openai/codex) inside isolated Docker containers with auth proxy, network separation, and parallel worker support.

## Features

- **Security isolation**: Codex runs in a Docker container, not on your host
- **Auth Proxy**: API keys never touch the container; short-lived tokens are injected instead
- **dock-net**: Dedicated Docker bridge network with ICC disabled and host access blocked
- **git worktree**: Parallel development branches, each in their own container
- **dock-ui**: Terminal UI for managing all workers at a glance
- **Package management**: `apt`, `pip`, `npm` packages via `--pkg` or `packages.dock`

## Requirements

### システム要件

| 項目 | 要件 | 備考 |
|------|------|------|
| OS | Linux (amd64 / arm64) または macOS (amd64 / arm64) | Windows は未サポート |
| Docker Engine | 20.10 以上 | Docker Desktop でも可 |
| git | 任意 | `--worktree` 機能を使う場合に必要 |

### ビルド要件

`go install` またはソースからビルドする場合のみ必要です。

| 項目 | 要件 |
|------|------|
| Go | 1.24 以上 |

Go のインストール: https://go.dev/doc/install

### 認証要件

以下のいずれかが必要です。

| 方式 | 取得先 |
|------|--------|
| **OpenAI API キー** (`OPENAI_API_KEY`) | [platform.openai.com/api-keys](https://platform.openai.com/api-keys) |
| **ChatGPT サブスクリプション** (Plus / Pro / Team / Business / Enterprise) | [chatgpt.com](https://chatgpt.com) でログイン後 `codex login` を実行 |

## Installation

### go install（推奨）

Go がインストール済みであれば、1 コマンドでインストールできます。

```bash
go install github.com/pacificbelt30/codex-dock@latest
```

インストール後、`$(go env GOPATH)/bin` が `PATH` に含まれていることを確認してください。

### Makefile を使う（ソースから一括セットアップ）

バイナリのインストール・デフォルト設定ファイルの配置・サンドボックスイメージのビルドを一括で行えます。

```bash
git clone https://github.com/pacificbelt30/codex-dock.git
cd codex-dock

# バイナリ + 設定ファイル + Docker イメージをまとめてセットアップ
make all             # バイナリをビルドし codex-dock:latest イメージを作成
sudo make install    # /usr/local/bin にバイナリを配置
make install-config  # ~/.config/codex-dock/config.toml を配置（既存の場合はスキップ）
```

または `install-all` で binary + config を一括インストール：

```bash
sudo make install-all
```

インストール先を変更したい場合は `PREFIX` を指定します：

```bash
sudo make install PREFIX=/opt/codex-dock
```

主要な Make ターゲット一覧：

| ターゲット | 内容 |
|---|---|
| `make build` | 現在のプラットフォーム向けにバイナリをビルド |
| `make install` | `$(PREFIX)/bin`（デフォルト `/usr/local/bin`）にバイナリを配置 |
| `make install-config` | `~/.config/codex-dock/config.toml` にデフォルト設定を配置 |
| `make install-all` | `install` + `install-config` を一括実行 |
| `make docker` | `codex-dock:latest` Docker イメージをビルド |
| `make all` | `build` + `docker`（デフォルトターゲット） |
| `make build-all` | darwin/arm64・darwin/amd64・linux/arm64 のクロスコンパイル |
| `make test` | テストスイートを実行（race detector + coverage） |
| `make lint` | golangci-lint を実行 |
| `make uninstall` | インストール済みバイナリを削除 |
| `make clean` | ビルド成果物を削除 |

### ソースからビルド（手動）

```bash
git clone https://github.com/pacificbelt30/codex-dock.git
cd codex-dock
go build -o codex-dock .

# PATH へ追加（任意）
sudo mv codex-dock /usr/local/bin/
```

---

## Quick Start

```bash
# 0. Auth Proxy を起動（必須）
codex-dock network create

docker build -t codex-dock-auth-proxy:latest -f docker/auth-proxy.Dockerfile .
docker run -d --name codex-auth-proxy --network dock-net \
  -p 127.0.0.1:18080:18080 \
  -e OPENAI_API_KEY="$OPENAI_API_KEY" \
  codex-dock-auth-proxy:latest

# （代替）ローカルプロセスで起動
# codex-dock proxy serve --listen 0.0.0.0:18080

# 1. サンドボックスイメージをビルド
codex-dock build

# 2. 認証を設定（API キーまたは ChatGPT OAuth）
export OPENAI_API_KEY=sk-...
codex-dock auth set

# 3. カレントディレクトリをマウントして起動
codex-dock run

# タスクを指定して全自動・バックグラウンド実行
codex-dock run --task "Write unit tests for auth module" --full-auto --detach

# git worktree を使ってブランチを切り離して作業
codex-dock run --worktree --branch feature-auth --new-branch

# 3 つのワーカーを並列実行
codex-dock run --parallel 3 --worktree --detach

# TUI でワーカーを管理（↑↓選択・Enter でログ・S 停止・R 再起動・D 削除）
codex-dock ui
```

`codex-dock run` は外部 Auth Proxy（デフォルト `http://127.0.0.1:18080`）へ接続します。
必要に応じて `--proxy-admin-url` / `--proxy-container-url` / `--proxy-admin-secret` を指定してください。

`connecting external auth proxy: connecting remote auth proxy ... connect: connection refused`
が出る場合は Auth Proxy が未起動です。先に上記の Step 0 を実行してください。

> **詳細な手順は [doc/getting-started.md](doc/getting-started.md) を参照してください。**

## Commands

| Command | Description |
|---------|-------------|
| `codex-dock run` | Start a sandboxed Codex worker |
| `codex-dock ps` | List workers |
| `codex-dock stop` | Stop containers |
| `codex-dock rm` | Remove stopped containers |
| `codex-dock logs` | View container logs |
| `codex-dock ui` | Launch TUI dashboard |
| `codex-dock auth` | Manage authentication |
| `codex-dock network` | Manage dock-net |
| `codex-dock build` | Build sandbox image |

## Documentation

| ドキュメント | 内容 |
|-------------|------|
| [Getting Started](doc/getting-started.md) | インストールから最初のコンテナ起動まで |
| [Architecture](doc/architecture.md) | システム構成・コンポーネント・起動フロー |
| [Auth Proxy](doc/auth-proxy.md) | 認証プロキシの技術仕様・エンドポイント |
| [Network](doc/network.md) | dock-net の構成・セキュリティポリシー |
| [Commands](doc/commands.md) | 全コマンドとオプションのリファレンス |
| [Configuration](doc/configuration.md) | `config.toml` の設定項目リファレンス |

## Configuration

Default config: `~/.config/codex-dock/config.toml`

See [`configs/config.toml.example`](configs/config.toml.example) for all options.

## Security

- Containers run as non-root (uid:1000)
- `--cap-drop ALL` applied
- `auth.json` is never mounted into containers
- Tokens expire on container stop or TTL expiry
- Container↔container and container→host traffic blocked via ICC and iptables

## Implementation Status

SRS v1.0.0 との実装差分を記録する。凡例: ✅ 実装済 / ⚠️ 部分実装 / ❌ 未実装

### 必須要件 (Priority: 必須)

| ID | 要件 | 状態 | 備考 |
|----|------|------|------|
| F-AUTH-01 | auth.json をコンテナに配置しない | ✅ | OAuth モードでも bind mount せず、Auth Proxy 経由で access_token のみを渡す |
| F-AUTH-02 | 本物の API Key をコンテナに渡さない | ✅ | |
| F-AUTH-03 | Auth Proxy が短命トークンを払い出す | ✅ | |
| F-AUTH-04 | コンテナ停止時にトークンを即時失効 | ✅ | `Manager.Stop()` / `StopByName()` でトークンを失効。コンテナ自然終了時も `Manager.RevokeToken()` で失効。 |
| F-AUTH-05 | トークンに TTL を設ける | ✅ | |
| F-NET-01 | dock-net 自動作成・ICC 無効化 | ✅ | |
| F-NET-02 | コンテナ→ホスト通信の遮断 | ⚠️ | Linux では `DOCKER-USER` + `iptables` で private/link-local 宛を遮断。root 権限が必要。macOS / Windows では同等の自動制御は未実装。 |
| F-NET-03 | コンテナ→インターネット通信を許可 | ✅ | IP masquerade 有効 |
| F-NET-04 | Auth Proxy への通信のみ許可 | ✅ | `authproxy.Config.ListenAddr` を追加し、`cmd/run.go` が dock-net ゲートウェイアドレス (`192.168.200.1:0`) で Auth Proxy を起動するよう変更。コンテナから `CODEX_AUTH_PROXY_URL` 経由で到達可能。 |
| F-NET-05 | `--no-internet` 対応 | ✅ | IP masquerade を false に設定 |
| F-PKG-01 | `--image` で任意イメージを指定 | ✅ | |
| F-PKG-02 | `--pkg` で追加パッケージを指定 | ✅ | |
| F-WT-01 | `--worktree` フラグ | ✅ | |
| F-WT-02 | `--branch` / `--new-branch` | ✅ | |
| F-WT-03 | worktree 自動作成・終了時自動削除 | ✅ | |
| F-UI-01 | コンテナ一覧リアルタイム更新 (2秒) | ✅ | |
| F-UI-02 | UI からコンテナ起動・停止・削除 | ✅ | `[R]` キーで `Manager.Start()` を呼び停止コンテナを再起動。停止(`S`)・削除(`D`)・全停止(`A`) も実装済み。 |
| F-UI-03 | UI からログ参照 | ✅ | Enter でログビューに遷移し、`mgr.Logs()` + `tviewWriter` で実際のログをリアルタイム表示。 |
| NF-SEC-01 | Auth Proxy 通信を TLS または UNIX ソケット | ❌ | 現在は平文 HTTP。SRS では暗号化が必須要件。UNIX ドメインソケットへの変更、または TLS の追加が必要。 |
| NF-SEC-02 | 非 root ユーザー (uid:1000) で実行 | ✅ | Dockerfile で `USER codex` |
| NF-SEC-03 | `--cap-drop ALL` | ✅ | |
| NF-SEC-04 | 認証情報をログ・標準出力に出力しない | ✅ | |
| NF-OPS-01 | Linux / macOS 両対応 | ✅ | CI でクロスコンパイル確認 |
| NF-OPS-02 | シングルバイナリ配布 | ✅ | |
| NF-OPS-03 | `~/.config/codex-dock/config.toml` | ✅ | |

### 推奨要件 (Priority: 推奨)

| ID | 要件 | 状態 | 備考 |
|----|------|------|------|
| F-AUTH-06 | コンテナ ID による Auth Proxy リクエスト認証 | ⚠️ | `X-Codex-Token` ヘッダーによる認証は実装済みだが、コンテナ ID との照合はない。なりすまし防止のためのコンテナ ID 検証は未実装。 |
| F-AUTH-07 | トークン払い出し・使用ログ | ⚠️ | `--verbose` 時のみ標準出力へ出力。永続的な監査ログは未実装。 |
| F-NET-06 | `network status` コマンド | ✅ | |
| F-PKG-03 | `--pkg-file` 対応 | ✅ | |
| F-PKG-04 | `packages.dock` 自動検出 | ✅ | |
| F-PKG-05 | apt / pip / npm プレフィックス自動判定 | ✅ | `detectManager()` を実装。`@scope/pkg` → npm、PEP 508 バージョン指定子 (`==`, `>=` 等) → pip、それ以外 → apt に自動分類。 |
| F-WT-04 | `--parallel N` 並列起動 | ✅ | |
| F-UI-04 | dock-net / Auth Proxy 状態をヘッダーに表示 | ❌ | ヘッダーにコンテナ数のみ表示。dock-net の有無・Auth Proxy の稼働状態は未表示（UI は Manager のみ持ち、proxy/network にアクセスできないため）。 |
| F-UI-05 | コンテナ詳細表示 (イメージ・マウント・TTL) | ❌ | 未実装 |
| NF-SEC-05 | `/proc`, `/sys`, `/dev` の最小マウント | ⚠️ | Docker デフォルトのマウントが適用される。明示的な制限は未設定。 |
| NF-SEC-06 | Seccomp プロファイル適用 | ❌ | `SecurityOpt` に Seccomp プロファイルを指定していない。`default` プロファイルは Docker デーモン設定依存。 |
| NF-OPS-04 | クラッシュ時のコンテナ残存防止 | ⚠️ | SIGINT / SIGTERM のシグナルハンドラは実装済み。プロセス強制終了時のクリーンアップ（`defer` 以外）は未実装。 |
| NF-OPS-05 | `--verbose` / `--debug` | ✅ | |

### 任意要件 (Priority: 任意)

| ID | 要件 | 状態 | 備考 |
|----|------|------|------|
| F-AUTH-08 | Auth Proxy レベルのレート制限 | ❌ | 未実装 |
| F-PKG-06 | `codex-dock build` | ✅ | |
| F-WT-05 | worktree 名・パスのユーザー指定 | ❌ | パスは自動決定のみ |
| F-UI-06 | CPU・メモリ使用量表示 | ❌ | 未実装 |

### その他の実装上の不具合

| 項目 | 内容 |
|------|------|
| ~~`--agents-md` が機能しない~~ | ✅ 修正済み: `manager.go` で `CODEX_AGENTS_MD` 環境変数をコンテナに渡すよう修正。 |
| ~~`_ = mountMode`~~ | ✅ 修正済み: dead code の `mountMode` 変数と `_ = mountMode` を削除。 |

---

### ChatGPT 定額プラン（サブスクリプション）への対応

Codex CLI は **API キー認証** と **ChatGPT アカウント認証（OAuth）** の 2 種類の認証方式をサポートする。ChatGPT Plus / Pro / Team / Business / Enterprise プランの契約者は、API クレジットなしに Codex を利用できる。

| Codex CLI 認証方式 | codex-dock 対応状況 | 方式 |
|-------------------|-------------------|------|
| API キー (`OPENAI_API_KEY`) | ✅ 対応 | Auth Proxy がキーをトークンに変換してコンテナに注入 |
| ChatGPT OAuth（ブラウザ / デバイスコードフロー） | ✅ 対応 | Auth Proxy が `access_token` のみをコンテナに渡す（F-AUTH-01 準拠） |

#### 実装方式: Auth Proxy 経由での access_token 注入

OAuth 認証時（`~/.codex/auth.json` に `refresh_token` フィールドが存在する場合）は、Auth Proxy がホスト側で OAuth 認証情報をメモリに保持する。コンテナには従来の API キーモードと同様に短命トークンを発行し、entrypoint.sh がそれを使って Auth Proxy から `access_token` のみを取得する。

```
ホスト (Auth Proxy)                  コンテナ (entrypoint.sh)
~/.codex/auth.json を読み込み        短命トークンで /token を叩く
  access_token ─────────────────▶  一時 auth.json を生成
  refresh_token (ホスト側のみ保持)    { access_token: "..." }
                                     ※ refresh_token は含まない
```

- **F-AUTH-01 準拠**: auth.json はコンテナに bind mount されない
- **refresh_token の保護**: コンテナは refresh_token を知らない
- **制限**: トークンの自動リフレッシュは現在非対応。セッションはアクセストークンの有効期限内に収める必要がある

> **参考**: [Codex CLI 認証ドキュメント](https://developers.openai.com/codex/auth/) / [ChatGPT プランでの Codex 利用](https://help.openai.com/en/articles/11369540-using-codex-with-your-chatgpt-plan)

---
