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
│   ├── authproxy/           Auth Proxy（認証プロキシ）
│   │   ├── proxy.go         HTTPサーバ・トークン管理
│   │   └── auth.go          APIキー/OAuthクレデンシャル読み込み
│   ├── sandbox/             Docker コンテナ管理
│   │   ├── manager.go       コンテナのライフサイクル
│   │   ├── types.go         RunOptions 等の型定義
│   │   └── packages.go      パッケージ定義解析
│   ├── network/             dock-net 管理
│   │   └── manager.go       ブリッジネットワークの作成/削除
│   ├── worktree/            git worktree 管理
│   │   └── worktree.go      worktree の作成/削除
│   └── ui/                  ターミナル UI (Bubble Tea)
│       └── ui.go
│
└── docker/
    ├── Dockerfile           サンドボックスイメージ定義
    └── entrypoint.sh        コンテナ起動スクリプト（認証取得含む）
```

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

## 認証モードの違い

codex-dock は **API キーモード** と **OAuth モード** の 2 つの認証方式をサポートします。

詳細なフロー図は [Auth Proxy 技術仕様](auth-proxy.md) を参照してください。

| モード | 条件 | 転送先 |
|---|---|---|
| API キーモード | `OPENAI_API_KEY` または `~/.config/codex-dock/apikey` がある場合 | `api.openai.com/v1` |
| OAuth モード | `~/.codex/auth.json` に `refresh_token` または `auth_mode: "chatgpt"` がある場合 | `chatgpt.com/backend-api/codex` |

---

## 関連ドキュメント

- [セキュリティ設計](security.md) — コンテナ設定・保護の仕組み・既知の問題
- [Auth Proxy 技術仕様](auth-proxy.md) — 認証プロキシの詳細仕様
- [ネットワーク仕様](network.md) — dock-net の構成・セキュリティポリシー
- [コマンドリファレンス: run](commands/run.md) — `codex-dock run` の全オプション
