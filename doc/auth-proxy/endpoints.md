# Auth Proxy — API エンドポイント仕様

> **日本語** | [English](../../en/auth-proxy/endpoints.md)

Auth Proxy が公開する全 HTTP エンドポイントの詳細仕様です。

- [概要・デプロイ](../auth-proxy.md)
- **API エンドポイント仕様** ← 本ページ
- [トークンの仕組みとセキュリティ](tokens.md)

---

## エンドポイント一覧

| エンドポイント | メソッド | 用途 |
|---|---|---|
| [`/token`](#get-token--クレデンシャル取得) | GET | コンテナがクレデンシャルを取得 |
| [`/oauth/token`](#post-oauthtoken--oauth-トークンリフレッシュ中継) | POST | OAuth トークンリフレッシュ中継 |
| [`/v1/*`](#any-v1--responses-api-リバースプロキシ) | ANY | Responses API リバースプロキシ |
| [`/chatgpt/*`](#any-chatgpt--chatgpt-backend-api-プロキシ) | ANY | ChatGPT backend-api プロキシ |
| [`/health`](#get-health--ヘルスチェック) | GET | ヘルスチェック |
| [`/revoke`](#post-revoke--トークン失効) | POST | トークン失効 |
| [`/admin/issue`](#post-adminissue--トークン発行管理) | POST | トークン発行（管理用） |
| [`/admin/revoke`](#post-adminrevoke--トークン失効管理) | POST | トークン失効（管理用） |
| [`/admin/mode`](#get-adminmode--動作モード確認) | GET | 動作モード確認 |

---

## `GET /token` — クレデンシャル取得

コンテナが短命トークンを使って認証情報を取得するエンドポイントです。
`entrypoint.sh` が起動時に呼び出します。

**リクエスト**

```
GET /token HTTP/1.1
X-Codex-Token: cdx-<64桁の16進数>
```

**レスポンス（API キーモード）**

```json
HTTP/1.1 200 OK
Content-Type: application/json

{
  "api_key": "cdx-a1b2c3d4...",
  "container_name": "codex-brave-atlas"
}
```

> `api_key` は本物の API キーではなく、`CODEX_TOKEN` と同じプレースホルダー値です。
> 本物の API キーはプロキシが送信時に `Authorization` ヘッダーへインジェクトします。

**レスポンス（OAuth モード）**

```json
HTTP/1.1 200 OK
Content-Type: application/json

{
  "oauth_access_token": "cdx-a1b2c3d4...",
  "oauth_id_token":     "eyJhbGci...",
  "oauth_account_id":   "user_xxx",
  "oauth_last_refresh": "2026-03-08T00:00:00Z",
  "container_name":     "codex-calm-beacon"
}
```

> - `oauth_access_token` は本物のアクセストークンではなく、`CODEX_TOKEN` と同じプレースホルダー値です。本物のアクセストークンはプロキシが送信時にインジェクトします。
> - `oauth_id_token` は本物の JWT です。Codex CLI が `chatgpt_account_id` や `chatgpt_plan_type` などの claims をローカルで読み取るために必要です（署名検証は行われません）。
> - `oauth_refresh_token` は返しません（セキュリティ上の設計）。

**エラーレスポンス**

| ステータス | 条件 |
|---|---|
| `401 Unauthorized` | `X-Codex-Token` ヘッダーが欠如、無効、または期限切れ |
| `405 Method Not Allowed` | GET 以外のメソッド |

---

## `POST /oauth/token` — OAuth トークンリフレッシュ中継

Codex CLI が `CODEX_REFRESH_TOKEN_URL_OVERRIDE` 経由でトークンリフレッシュを要求するエンドポイントです。
プロキシは `refresh_token` をホストのものに差し替えて `https://auth.openai.com/oauth/token` に転送します。

**認証**: クエリパラメーター `?cdx=<短命トークン>` で行います（Codex CLI はリフレッシュリクエストにカスタムヘッダーを付けないため）。

**Codex CLI からのリクエスト形式**

```
POST /oauth/token?cdx=<cdx-xxxx> HTTP/1.1
Content-Type: application/json

{
  "client_id":    "app_EMoamEEZ73f0CkXaXp7hrann",
  "grant_type":   "refresh_token",
  "refresh_token": ""
}
```

**プロキシが OpenAI に送る形式**

プロキシは `refresh_token` フィールドのみをホストの本物の値に差し替え、他のフィールドはそのまま転送します。

```
POST https://auth.openai.com/oauth/token HTTP/1.1
Content-Type: application/json

{
  "client_id":    "app_EMoamEEZ73f0CkXaXp7hrann",
  "grant_type":   "refresh_token",
  "refresh_token": "<ホストの本物の refresh_token>"
}
```

**監視・変更フィールド一覧**

| フィールド | 処理 | 理由 |
|---|---|---|
| `refresh_token` (リクエスト) | **ホストの値に差し替え** | コンテナの `refresh_token` は空文字のため |
| `client_id` | そのまま通過 | Codex CLI が自身でハードコード済み |
| `grant_type` | そのまま通過 | 変更不要 |
| その他フィールド | そのまま通過 | 変更不要 |
| `refresh_token` (レスポンス) | **レスポンスから除外** | コンテナに新しい refresh_token を渡さない |
| `access_token` (レスポンス) | **プレースホルダーに差し替え** | 本物の新 access_token をコンテナに渡さない |
| `id_token` (レスポンス) | そのまま通過 | claims 抽出用（認証 credential ではない） |
| その他レスポンスフィールド | そのまま通過 | — |

**エラーレスポンス**

| ステータス | 条件 |
|---|---|
| `400 Bad Request` | OAuth モードでない |
| `401 Unauthorized` | `cdx` パラメーターが欠如・無効・期限切れ |
| `405 Method Not Allowed` | POST 以外のメソッド |
| `502 Bad Gateway` | OpenAI への転送失敗 |

---

## `ANY /v1/*` — Responses API リバースプロキシ

Codex CLI は `OPENAI_BASE_URL=http://proxy/v1` を参照し、すべての Responses API リクエストをプロキシ経由で送ります。

| 認証モード | 転送先 |
|---|---|
| API キー | `https://api.openai.com/v1/<path>` |
| OAuth / ChatGPT | `https://chatgpt.com/backend-api/codex/<path>` |

- `Authorization` ヘッダーはコンテナのプレースホルダー値を**ホストの本物のクレデンシャルで上書き**します
- OAuth モードでは `ChatGPT-Account-Id` ヘッダーも `oauthCreds.AccountID` の正しい値で上書きします
- hop-by-hop ヘッダー（`Connection`・`Transfer-Encoding` 等）は除去します
- レスポンスのステータス・ヘッダー・ボディをそのままコンテナに返します
- WebSocket アップグレードリクエストも同様に `Authorization` と `ChatGPT-Account-Id` を差し替えてトンネリングします

**上流への実際のヘッダー（プロキシが差し替え後）**

```
Authorization: Bearer <本物の access_token または api_key>  ← プロキシがインジェクト
Content-Type: application/json
chatgpt-account-id: <account_id>   ← OAuth時: プロキシが正値で上書き
```

---

## `ANY /chatgpt/*` — ChatGPT backend-api プロキシ

`/chatgpt/` 以下を `https://chatgpt.com/backend-api/` に転送します。
OAuth モードのコンテナは `~/.config/codex/config.toml` の `chatgpt_base_url=http://proxy/chatgpt/` 経由でレート制限情報・アカウント情報取得リクエストをここへ送ります。

---

## `GET /health` — ヘルスチェック

```json
HTTP/1.1 200 OK
Content-Type: application/json

{
  "status": "ok",
  "active_tokens": 3
}
```

---

## `POST /revoke` — トークン失効

コンテナ停止時に `sandbox.Manager.Stop()` が呼び出します。

```
POST /revoke?container=<コンテナ名> HTTP/1.1
```

| ステータス | 条件 |
|---|---|
| `200 OK` | 失効成功 |
| `400 Bad Request` | `container` パラメーターが欠如 |
| `405 Method Not Allowed` | POST 以外のメソッド |

---

## 管理用エンドポイント（`/admin/*`）

管理用エンドポイントは `X-Proxy-Admin-Secret` ヘッダーによる認証が必要です（`--admin-secret` が設定されている場合）。
`codex-dock proxy run --admin-secret <シークレット>` で起動した場合に利用できます。

### `POST /admin/issue` — トークン発行（管理用）

任意のコンテナ名で短命トークンを発行します。
`codex-dock run` を使わずに Auth Proxy を利用する際に使用します。
→ 詳細は [Auth Proxy のみを使う](../proxy-standalone.md) を参照してください。

**リクエスト**

```
POST /admin/issue HTTP/1.1
Content-Type: application/json
X-Proxy-Admin-Secret: <シークレット>

{
  "container": "my-session",
  "ttl": 3600
}
```

| フィールド | 必須 | 説明 |
|---|---|---|
| `container` | ✅ | コンテナ（セッション）名 |
| `ttl` | | TTL（秒）。省略時はプロキシの `default_token_ttl` を使用 |

**レスポンス**

```json
HTTP/1.1 200 OK
Content-Type: application/json

{"token": "cdx-a1b2c3d4..."}
```

**エラーレスポンス**

| ステータス | 条件 |
|---|---|
| `400 Bad Request` | `container` が空、または JSON が不正 |
| `401 Unauthorized` | `X-Proxy-Admin-Secret` が不正 |
| `405 Method Not Allowed` | POST 以外のメソッド |

---

### `POST /admin/revoke` — トークン失効（管理用）

指定コンテナのトークンを即時失効させます。

```
POST /admin/revoke?container=<コンテナ名> HTTP/1.1
X-Proxy-Admin-Secret: <シークレット>
```

---

### `GET /admin/mode` — 動作モード確認

現在の認証モードを確認します。

```json
HTTP/1.1 200 OK
Content-Type: application/json

{"oauth_mode": false}
```

---

## 関連ドキュメント

- [Auth Proxy 概要・デプロイ](../auth-proxy.md)
- [トークンの仕組みとセキュリティ](tokens.md)
- [Auth Proxy のみを使う（Codex 設定ガイド）](../proxy-standalone.md)
- [ネットワーク仕様](../network.md)
