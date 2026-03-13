# Auth Proxy 技術仕様

> **日本語** | [English](en/auth-proxy.md)

- **概要・デプロイ** ← 本ページ
- [API エンドポイント仕様](auth-proxy/endpoints.md)
- [トークンの仕組みとセキュリティ](auth-proxy/tokens.md)

---

Auth Proxy は codex-dock のセキュリティの核となるコンポーネントです。
コンテナに実際の API キーや OAuth クレデンシャルを渡さず、短命トークンを介して安全に認証情報を提供します。
Codex CLI が呼ぶすべての OpenAI API トラフィックをプロキシし、**コンテナが保持するのはプレースホルダートークンのみ**とすることで、本物のクレデンシャルがコンテナに届かない構造を実現します。

---

## デプロイ方式

Auth Proxy は `codex-dock proxy run` で Docker コンテナとして起動するか、`codex-dock proxy serve` でローカルプロセスとして起動します。

```bash
# Docker コンテナとして起動（推奨）
codex-dock proxy run --admin-secret <シークレット>

# ローカルプロセスとして起動
codex-dock proxy serve --listen 0.0.0.0:18080 --admin-secret <シークレット>
```

| 接続先 | URL |
|---|---|
| ホスト側管理 API | `http://127.0.0.1:18080` |
| コンテナからの到達先 | `http://codex-auth-proxy:18080`（Docker ネットワーク経由）または `http://host.docker.internal:PORT` |

---

## 構成図

```
┌──────────────────────────────────────────────────────────────────────┐
│  ホスト環境                                                            │
│                                                                        │
│  ~/.codex/auth.json         ~/.config/codex-dock/apikey               │
│  (OAuth クレデンシャル)      (API キー)                                │
│          │                           │                                 │
│          └───────────┬───────────────┘                                 │
│                      ▼                                                 │
│  ┌──────────────────────────────────────────────────────────────────┐ │
│  │  Auth Proxy (0.0.0.0:PORT)                                       │ │
│  │                                                                  │ │
│  │  GET  /token        トークン検証 → クレデンシャル返却            │ │
│  │  POST /oauth/token  OAuthトークンリフレッシュ中継                │ │
│  │  ANY  /v1/*         Responses API リバースプロキシ               │ │
│  │  ANY  /chatgpt/*    ChatGPT backend-api プロキシ                 │ │
│  │  GET  /health       ヘルスチェック                               │ │
│  │  POST /revoke       トークン失効                                 │ │
│  │  POST /admin/issue  トークン発行（管理用）                       │ │
│  └──────────────────────────────────────────────────────────────────┘ │
│                      │ 短命トークン (cdx-xxxx)                         │
│                      ▼                                                 │
│  ┌──────────────────────────────────────────────────────────────────┐ │
│  │  サンドボックスコンテナ                                           │ │
│  │  CODEX_TOKEN, OPENAI_BASE_URL, CODEX_REFRESH_TOKEN_URL_OVERRIDE  │ │
│  └──────────────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────────────┘
```

---

## 認証モード

### API キーモード

`OPENAI_API_KEY` 環境変数または `~/.config/codex-dock/apikey` ファイルから API キーを読み込みます。

```
ホスト (Auth Proxy)                          コンテナ (entrypoint.sh)
  │                                                │
  │  短命トークン発行: CODEX_TOKEN=cdx-<hex64>    │
  │  env: OPENAI_BASE_URL=http://proxy/v1         │
  │ ───────────────────────────────────────────▶  │
  │                                                │ 起動時
  │  GET /token (X-Codex-Token: cdx-<hex64>)      │
  │  ◀─────────────────────────────────────────── │
  │                                                │
  │  200 OK {"api_key": "cdx-<hex64>"}  ← プレースホルダー
  │ ───────────────────────────────────────────▶  │
  │                                                │ export OPENAI_API_KEY=cdx-<hex64>
  │                                                │ exec codex
  │                                                │
  │  POST /v1/responses (Authorization: Bearer cdx-<hex64>)
  │  ◀─────────────────────────────────────────── │
  │  Authorization を差し替え: Bearer sk-...      │ ← プロキシがインジェクト
  │  転送先: https://api.openai.com/v1/responses  │
```

### OAuth モード（ChatGPT サブスクリプション）

`~/.codex/auth.json` に `refresh_token` フィールドがある、または `auth_mode: "chatgpt"` が設定されている場合に自動的に OAuth モードで動作します。

```
ホスト (Auth Proxy)                          コンテナ (entrypoint.sh)
  │                                                │
  │  短命トークン発行: CODEX_TOKEN=cdx-<hex64>    │
  │  env: OPENAI_BASE_URL=http://proxy/v1         │
  │  env: CODEX_REFRESH_TOKEN_URL_OVERRIDE=        │
  │       http://proxy/oauth/token?cdx=<token>    │
  │ ───────────────────────────────────────────▶  │
  │                                                │ GET /token
  │  200 OK                                        │
  │  {"oauth_access_token": "cdx-<hex64>",        │ ← プレースホルダー
  │   "oauth_id_token":     "ey...",              │ ← 本物 (claims 抽出用)
  │   "oauth_account_id":   "..."}                │
  │ ───────────────────────────────────────────▶  │
  │                                                │ ~/.codex/auth.json 生成
  │                                                │   access_token: "cdx-<hex64>"
  │                                                │   refresh_token: ""
  │                                                │ ~/.config/codex/config.toml
  │                                                │   chatgpt_base_url=http://proxy/chatgpt/
  │                                                │ exec codex
  │                                                │
  │  POST /v1/responses (Authorization: Bearer cdx-<hex64>)
  │  ◀─────────────────────────────────────────── │
  │  Authorization を本物の access_token に差し替え
  │  転送先: https://chatgpt.com/backend-api/codex/responses
```

> **セキュリティ**: `refresh_token` および本物の `access_token` はコンテナに渡されません。
> コンテナが保持するのは CODEX_TOKEN と同一のプレースホルダーのみで、プロキシがすべての送信リクエストの Authorization ヘッダーを本物の access_token で差し替えます。

---

## 関連ドキュメント

- [API エンドポイント仕様](auth-proxy/endpoints.md) — 全エンドポイントの詳細
- [トークンの仕組みとセキュリティ](auth-proxy/tokens.md) — トークンライフサイクル・セキュリティ考慮事項
- [Auth Proxy のみを使う](proxy-standalone.md) — codex-dock run を使わずに Codex CLI を設定する方法
- [ネットワーク仕様](network.md) — dock-net とホスト到達性
- [コマンドリファレンス: proxy](commands/proxy.md) — `codex-dock proxy` コマンドの詳細
