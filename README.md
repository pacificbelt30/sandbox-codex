# codex-dock

**AI Sandbox Container Manager** — Runs [Codex CLI](https://github.com/openai/codex) inside isolated Docker containers with auth proxy, network separation, and parallel worker support.

## Features

- **Security isolation**: Codex runs in a Docker container, not on your host
- **Auth Proxy**: API keys never touch the container; short-lived tokens are injected instead
- **dock-net**: Dedicated Docker bridge network with ICC disabled and host access blocked
- **git worktree**: Parallel development branches, each in their own container
- **dock-ui**: Terminal UI for managing all workers at a glance
- **Package management**: `apt`, `pip`, `npm` packages via `--pkg` or `packages.dock`

## Quick Start

```bash
# Build the base sandbox image
codex-dock build

# Set your API key
export OPENAI_API_KEY=sk-...
codex-dock auth set

# Run Codex in a sandbox (mounts current directory)
codex-dock run

# Run with a task, fully automated, in the background
codex-dock run --task "Write unit tests for auth module" --full-auto --detach

# Use git worktree on a feature branch
codex-dock run --worktree --branch feature-auth --new-branch

# Parallel workers (3 branches auto-created)
codex-dock run --parallel 3 --worktree

# Monitor all workers
codex-dock ui
```

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

## Configuration

Default config: `~/.config/codex-dock/config.toml`

See `configs/config.toml.example` for all options.

## Security

- Containers run as non-root (uid:1000)
- `--cap-drop ALL` applied
- `auth.json` is never mounted into containers
- Tokens expire on container stop or TTL expiry
- Container↔container and container→host traffic blocked via ICC and iptables

## Requirements

- Go 1.22+
- Docker Engine

## Build

```bash
go build -o codex-dock .
```

---

## Implementation Status

SRS v1.0.0 との実装差分を記録する。凡例: ✅ 実装済 / ⚠️ 部分実装 / ❌ 未実装

### 必須要件 (Priority: 必須)

| ID | 要件 | 状態 | 備考 |
|----|------|------|------|
| F-AUTH-01 | auth.json をコンテナに配置しない | ✅ | OAuth モードでも bind mount せず、Auth Proxy 経由で access_token のみを渡す |
| F-AUTH-02 | 本物の API Key をコンテナに渡さない | ✅ | |
| F-AUTH-03 | Auth Proxy が短命トークンを払い出す | ✅ | |
| F-AUTH-04 | コンテナ停止時にトークンを即時失効 | ❌ | `proxy.RevokeToken()` は実装済みだが、`sandbox.Manager.Stop()` 経由でも `cmd/run.go` の停止フローでも呼ばれていない。コンテナ停止後もトークンが TTL 切れまで有効な状態になる。 |
| F-AUTH-05 | トークンに TTL を設ける | ✅ | |
| F-NET-01 | dock-net 自動作成・ICC 無効化 | ✅ | |
| F-NET-02 | コンテナ→ホスト通信の遮断 | ⚠️ | `enable_icc=false` でコンテナ間通信は遮断済み。ただしコンテナ→ホストを遮断する iptables ルールは未実装。完全な遮断には `coreos/go-iptables` による追加ルールが必要。 |
| F-NET-03 | コンテナ→インターネット通信を許可 | ✅ | IP masquerade 有効 |
| F-NET-04 | Auth Proxy への通信のみ許可 | ❌ | Auth Proxy がホストの loopback (`127.0.0.1`) でリッスンしているため、dock-net 上のコンテナから到達不可。SRS では Auth Proxy は dock-net 内の特定アドレスでリッスンする設計。コンテナへの `CODEX_AUTH_PROXY_URL` は現状機能しない。 |
| F-NET-05 | `--no-internet` 対応 | ✅ | IP masquerade を false に設定 |
| F-PKG-01 | `--image` で任意イメージを指定 | ✅ | |
| F-PKG-02 | `--pkg` で追加パッケージを指定 | ✅ | |
| F-WT-01 | `--worktree` フラグ | ✅ | |
| F-WT-02 | `--branch` / `--new-branch` | ✅ | |
| F-WT-03 | worktree 自動作成・終了時自動削除 | ✅ | |
| F-UI-01 | コンテナ一覧リアルタイム更新 (2秒) | ✅ | |
| F-UI-02 | UI からコンテナ起動・停止・削除 | ⚠️ | 停止(`S`)・削除(`D`)・全停止(`A`) は実装済み。フッターに表示される `[R] 起動` キーのハンドラが未実装。 |
| F-UI-03 | UI からログ参照 | ⚠️ | Enter でログビューに遷移するが、実際のログを `mgr.Logs()` で取得せずスタブテキストを表示している。 |
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
| F-PKG-05 | apt / pip / npm プレフィックス自動判定 | ⚠️ | プレフィックス省略時は常に `apt` に分類。パッケージ名から自動判定するロジックは未実装。 |
| F-WT-04 | `--parallel N` 並列起動 | ✅ | |
| F-UI-04 | dock-net / Auth Proxy 状態をヘッダーに表示 | ❌ | ヘッダーにコンテナ数のみ表示。dock-net の有無・Auth Proxy の稼働状態は未表示。 |
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
| `--agents-md` が機能しない | `RunOptions.AgentsMD` は受け取るが、`sandbox/manager.go` で `CODEX_AGENTS_MD` 環境変数としてコンテナに渡していない。`entrypoint.sh` 側のハンドラは実装済みのため、環境変数の設定だけが抜けている。 |
| `_ = mountMode` | `ReadOnly` の適用が `hostConfig.Mounts[0].ReadOnly` 経由で行われており、`mountMode` 変数が dead code になっている。 |

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
