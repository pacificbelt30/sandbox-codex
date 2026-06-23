# アーキテクチャ概要

> **日本語** | [English](en/architecture.md)

codex-dock は大きく **4 つのコンポーネント**で構成されています。

---

## コンポーネント構成

```
codex-dock
├── cmd/                     CLIコマンド (cobra)
│   ├── run.go               コンテナ起動・ワーカー管理
│   ├── auth.go              認証設定 (auth set / auth status)
│   ├── build.go             サンドボックスイメージのビルド
│   ├── ps.go / stop.go / rm.go / logs.go
│   ├── ui.go                TUI ダッシュボード起動
│   └── network.go           dock-net の管理
│
├── internal/
│   ├── authproxy/           Auth Proxy（認証プロキシ）★プロキシ側
│   │   ├── proxy.go         HTTPサーバ・トークン管理・OpenAI/Anthropic リバースプロキシ
│   │   ├── auth.go          OpenAI/Anthropic の APIキー/OAuth クレデンシャル読み込み
│   │   ├── remote.go        RemoteProxy（プロキシコンテナへのクライアント）
│   │   └── service.go       Service インターフェース
│   ├── sandbox/             Docker コンテナ管理 ★サンドボックス側
│   │   ├── manager.go       コンテナのライフサイクル・buildEnv（エージェント別）
│   │   ├── types.go         RunOptions / Agent(codex/claude/shell) 等の型定義
│   │   └── packages.go      パッケージ定義解析
│   ├── network/             dock-net 管理
│   │   └── manager.go       ブリッジネットワークの作成/削除
│   ├── worktree/            git worktree 管理
│   │   └── worktree.go      worktree の作成/削除
│   └── ui/                  ターミナル UI (Bubble Tea)
│       └── ui.go
│
└── docker/                  コンテナ資産（用途別に分離）
    ├── sandbox/             ★サンドボックスイメージ
    │   ├── Dockerfile       Node.js 22 + Codex CLI + Claude Code
    │   └── entrypoint.sh    起動スクリプト（認証取得・codex/claude/shell 分岐）
    └── proxy/               ★プロキシイメージ
        └── Dockerfile       Auth Proxy（Go バイナリ）
```

> **プロキシ／サンドボックスの分離**: 認証情報を扱うロジックは `internal/authproxy` + `docker/proxy`
> に、コンテナ実行ロジックは `internal/sandbox` + `docker/sandbox` に閉じています。両者は
> プロキシの HTTP API（`Service` インターフェース）経由でのみ通信し、コンテナ単位でも
> イメージ単位でも独立してビルド・デプロイできます。

---

## 起動シーケンス

`codex-dock run` を実行した際の処理フローを示します。

```
ユーザー           codex-dock CLI          Auth Proxy              Docker / コンテナ
  │                    │                       │                         │
  │  codex-dock run    │                       │                         │
  │──────────────────▶│                       │                         │
  │                    │                       │                         │
  │                    │ 1. dock-net 確認/作成  │                         │
  │                    │──────────────────────────────────────────────▶ │
  │                    │                       │                         │
  │                    │ 2. Auth Proxy に接続   │                         │
  │                    │──────────────────────▶│                         │
  │                    │                       │                         │
  │                    │ 3. 短命トークン発行    │                         │
  │                    │◀── cdx-xxxx...        │                         │
  │                    │                       │                         │
  │                    │ 4. コンテナ作成・起動  │                         │
  │                    │  CODEX_TOKEN=cdx-xxx  │                         │
  │                    │  OPENAI_BASE_URL=proxy │                         │
  │                    │──────────────────────────────────────────────▶ │
  │                    │                       │                         │
  │                    │                       │  5. GET /token          │
  │                    │                       │◀────────────────────────│
  │                    │                       │─────────────────────── ▶│
  │                    │                       │  {api_key or oauth...}  │
  │                    │                       │                         │
  │                    │                       │  6. Codex CLI 起動      │
  │                    │                       │                         │
  │                    │                       │  7. POST /v1/responses  │
  │                    │                       │◀────────────────────────│
  │                    │                       │  Authorization 差し替え  │
  │                    │                       │  転送先: api.openai.com  │
  │◀──────────────────────────────────────────────────────────────────── │
  │  コンテナ出力       │                       │                         │
```

---

## エージェントと認証モードの違い

`--agent` でサンドボックスが起動するエージェントを選びます（省略時は両 CLI が使える認証済みシェル）。

| `--agent` | エージェント | プロキシ転送先 |
|---|---|---|
| `codex` | OpenAI Codex CLI | `/v1/*` → OpenAI / ChatGPT |
| `claude` | Anthropic Claude Code | `/anthropic/*` → `api.anthropic.com` |

各プロバイダは **API キーモード** と **OAuth モード** をサポートします。
詳細なフロー図は [Auth Proxy 技術仕様](auth-proxy.md) を参照してください。

| プロバイダ / モード | 条件 | 転送先 |
|---|---|---|
| OpenAI API キー | `OPENAI_API_KEY` または `~/.config/codex-dock/apikey` | `api.openai.com/v1` |
| OpenAI OAuth | `~/.codex/auth.json` に `refresh_token` / `auth_mode: "chatgpt"` | `chatgpt.com/backend-api/codex` |
| Anthropic API キー | `ANTHROPIC_API_KEY` または `~/.config/codex-dock/anthropic-apikey` | `api.anthropic.com`（`x-api-key`） |
| Anthropic OAuth | `~/.claude/.credentials.json`（`claudeAiOauth`） | `api.anthropic.com`（`Authorization: Bearer` + `anthropic-beta`） |

---

## 関連ドキュメント

- [セキュリティ設計](security.md) — コンテナ設定・保護の仕組み・既知の問題
- [Auth Proxy 技術仕様](auth-proxy.md) — 認証プロキシの詳細仕様
- [ネットワーク仕様](network.md) — dock-net の構成・セキュリティポリシー
- [コマンドリファレンス: run](commands/run.md) — `codex-dock run` の全オプション
