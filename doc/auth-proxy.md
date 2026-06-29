# Auth Proxy 技術仕様

> **日本語** | [English](en/auth-proxy.md)

- **概要・デプロイ** ← 本ページ
- [API エンドポイント仕様](auth-proxy/endpoints.md)
- [トークンの仕組みとセキュリティ](auth-proxy/tokens.md)

---

Auth Proxy は codex-dock のセキュリティの核となるコンポーネントです。
コンテナに実際の API キーや OAuth クレデンシャルを渡さず、短命トークンを介して安全に認証情報を提供します。
**OpenAI（Codex）と Anthropic（Claude Code）の両方**に対応し、エージェントが呼ぶすべての API トラフィックをプロキシして、**コンテナが保持するのはプレースホルダートークンのみ**とすることで、本物のクレデンシャルがコンテナに届かない構造を実現します。1 つのプロキシで両プロバイダの認証情報を独立に読み込み、両エージェントに同時にサービスできます。

---

## デプロイ方式

Auth Proxy は `codex-dock proxy run` で Docker コンテナとして起動するか、`codex-dock proxy serve` でローカルプロセスとして起動します。

```bash
# Docker コンテナとして起動（推奨）
codex-dock proxy run --admin-secret <シークレット>

# ローカルプロセスとして起動（admin を別ポートに分離する例）
codex-dock proxy serve --listen 0.0.0.0:18080 --admin-listen 0.0.0.0:18081 --admin-secret <シークレット>
```

| 接続先 | URL |
|---|---|
| ホスト側管理 API（admin リスナー） | `http://127.0.0.1:18081` |
| コンテナからの到達先（データプレーン） | `http://codex-auth-proxy:18080`（各ワーカー専用 Internal ネットワーク上の Docker DNS 経由） |

> `proxy run` は admin ポート（既定 18081）のみをホスト loopback に公開し、データプレーン（18080）は非公開です。ワーカーは Docker DNS（`codex-auth-proxy`）でデータプレーンにのみ到達します。`/admin/*` はワーカーから到達できません。

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
│  │  ANY  /v1/*         OpenAI Responses API リバースプロキシ        │ │
│  │  ANY  /chatgpt/*    ChatGPT backend-api プロキシ                 │ │
│  │  ANY  /anthropic/*  Anthropic Messages API リバースプロキシ      │ │
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

### Anthropic モード（Claude Code）

Claude Code は完全に環境変数駆動です。`codex-dock run --agent claude` でコンテナに以下を注入します。

- `ANTHROPIC_BASE_URL=http://<proxy>/anthropic`
- `ANTHROPIC_API_KEY=cdx-<hex64>`（プレースホルダー、CODEX_TOKEN と同一）

プロキシは `/anthropic/*` への全リクエストでホストの本物のクレデンシャルを注入します。
認証情報は OpenAI とは独立に読み込まれます（優先順位: 環境変数 > 保存ファイル > OAuth）。

| ホストの認証種別 | 認証情報ソース | プロキシが注入するヘッダー |
|---|---|---|
| API キー | `ANTHROPIC_API_KEY` 環境変数 / `~/.config/codex-dock/anthropic-apikey` | `x-api-key: sk-ant-…`（`Authorization` は削除） |
| OAuth サブスクリプション | `~/.claude/.credentials.json`（`claudeAiOauth`） | `Authorization: Bearer …` + `anthropic-beta: oauth-2025-04-20`（`x-api-key` は削除） |

```
コンテナ (claude)                          ホスト (Auth Proxy)
  │  POST /anthropic/v1/messages                  │
  │  x-api-key: cdx-<hex64>  ← プレースホルダー    │
  │ ────────────────────────────────────────────▶ │
  │                          x-api-key を本物に差し替え（API キーモード）
  │                          または Authorization: Bearer + anthropic-beta（OAuth モード）
  │                          転送先: https://api.anthropic.com/v1/messages
```

> **OAuth トークンのリフレッシュ**: アクセストークンの有効期限が近い場合、プロキシが
> `refresh_token` を使って自前でリフレッシュします（`refreshAnthropicOAuthIfNeeded`）。
> `refresh_token` はホストに留まり、コンテナには一切渡りません。Codex の OAuth フローと同じ原則です。

---

## 関連ドキュメント

- [API エンドポイント仕様](auth-proxy/endpoints.md) — 全エンドポイントの詳細
- [トークンの仕組みとセキュリティ](auth-proxy/tokens.md) — トークンライフサイクル・セキュリティ考慮事項
- [Auth Proxy のみを使う](proxy-standalone.md) — codex-dock run を使わずに Codex CLI を設定する方法
- [ネットワーク仕様](network.md) — プロキシルータと per-worker Internal ネットワーク
- [コマンドリファレンス: proxy](commands/proxy.md) — `codex-dock proxy` コマンドの詳細
