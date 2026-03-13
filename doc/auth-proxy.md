# Auth Proxy 技術仕様

> **日本語** | [English](en/auth-proxy.md)

## デプロイ方式（現在）

Auth Proxy は `codex-dock run` プロセス内ではなく、**Docker コンテナとして外部常駐**させる前提です。

- 管理 API（ホスト側）: `http://127.0.0.1:18080`
- ワーカーからの到達先: `http://codex-auth-proxy:18080`
- `codex-dock run` は `--proxy-admin-url` / `--proxy-container-url` / `--proxy-admin-secret` で接続先を調整できます。

---

Auth Proxy は codex-dock のセキュリティの核となるコンポーネントです。
コンテナに実際の API キーや OAuth クレデンシャルを渡さず、短命トークンを介して安全に認証情報を提供します。
また、Codex CLI が呼ぶすべての OpenAI API トラフィック（Responses API・トークンリフレッシュ・ChatGPT backend-api）をプロキシし、**コンテナが保持するのはプレースホルダートークンのみ**とすることで、本物のクレデンシャルがコンテナに届かない構造を実現します。

---

## 概要

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
│  │  Auth Proxy (<dock-net-gateway>:PORT)      ← ランダムポート      │ │
│  │                                                                  │ │
│  │  GET  /token        トークン検証 → クレデンシャル返却            │ │
│  │  GET  /health       ヘルスチェック                               │ │
│  │  POST /revoke       トークン失効                                 │ │
│  │  POST /oauth/token  OAuthトークンリフレッシュ中継                │ │
│  │  ANY  /v1/*         Responses API リバースプロキシ               │ │
│  │  ANY  /chatgpt/*    ChatGPT backend-api プロキシ                 │ │
│  └──────────────────────────────────────────────────────────────────┘ │
│                      │ 短命トークン (cdx-xxxx)                         │
│                      ▼                                                 │
│  ┌──────────────────────────────────────────────────────────────────┐ │
│  │  サンドボックスコンテナ                                           │ │
│  │  CODEX_TOKEN                                                     │ │
│  │  CODEX_AUTH_PROXY_URL                                            │ │
│  │  OPENAI_BASE_URL            ← /v1 プロキシを向く                 │ │
│  │  CODEX_REFRESH_TOKEN_URL_OVERRIDE (OAuth時のみ)                  │ │
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
  │  短命トークン発行                               │
  │  env: CODEX_TOKEN=cdx-<hex64>                 │
  │  env: OPENAI_BASE_URL=http://proxy/v1         │
  │ ───────────────────────────────────────────▶  │
  │                                                │ 起動時
  │  GET /token                                    │
  │  X-Codex-Token: cdx-<hex64>                   │
  │  ◀─────────────────────────────────────────── │
  │                                                │
  │  200 OK {"api_key": "cdx-<hex64>"}  ← プレースホルダー
  │ ───────────────────────────────────────────▶  │
  │                                                │
  │                                                │ export OPENAI_API_KEY=cdx-<hex64>  ← ダミー値
  │                                                │ unset CODEX_TOKEN
  │                                                │ exec codex
  │                                                │
  │  POST /v1/responses ← Codex CLI               │
  │  Authorization: Bearer cdx-<hex64>  ← ダミー  │
  │  ◀─────────────────────────────────────────── │
  │                                                │
  │  Authorization を差し替え: Bearer sk-...      │ ← プロキシがインジェクト
  │  転送先: https://api.openai.com/v1/responses  │
  │ ─────────────────────────────────────────────▶│ (OpenAI)
```

### OAuth モード（ChatGPT サブスクリプション）

`~/.codex/auth.json` に `refresh_token` フィールドがある、または `auth_mode: "chatgpt"` が設定されている場合に自動的に OAuth モードで動作します。

```
ホスト (Auth Proxy)                          コンテナ (entrypoint.sh)
  │                                                │
  │  ~/.codex/auth.json より読み込み:              │
  │   access_token, id_token,                     │
  │   refresh_token (ホスト内のみ保持), account_id│
  │                                                │
  │  短命トークン発行                               │
  │  env: CODEX_TOKEN=cdx-<hex64>                 │
  │  env: OPENAI_BASE_URL=http://proxy/v1         │
  │  env: CODEX_REFRESH_TOKEN_URL_OVERRIDE=       │
  │       http://proxy/oauth/token?cdx=<token>    │
  │ ───────────────────────────────────────────▶  │
  │                                                │ 起動時
  │  GET /token                                    │
  │  X-Codex-Token: cdx-<hex64>                   │
  │  ◀─────────────────────────────────────────── │
  │                                                │
  │  200 OK                                        │
  │  {"oauth_access_token": "cdx-<hex64>",        │ ← プレースホルダー (CODEX_TOKEN と同じ値)
  │   "oauth_id_token":     "ey...",              │ ← 本物 (claims 抽出用)
  │   "oauth_account_id":   "...",                │
  │   "oauth_last_refresh": "..."}                │
  │  ※ oauth_access_token は本物でない / oauth_refresh_token は含まない
  │ ───────────────────────────────────────────▶  │
  │                                                │
  │                                                │ /home/codex/.codex/auth.json 生成
  │                                                │   access_token: "cdx-<hex64>"  ← ダミー
  │                                                │   refresh_token: "" (空)
  │                                                │ /home/codex/.config/codex/config.toml
  │                                                │   chatgpt_base_url=http://proxy/chatgpt/
  │                                                │ unset CODEX_TOKEN
  │                                                │ exec codex
  │                                                │
  │  POST /v1/responses ← Codex CLI               │
  │  Authorization: Bearer cdx-<hex64>  ← ダミー  │
  │  ChatGPT-Account-Id: <account_id>             │
  │  ◀─────────────────────────────────────────── │
  │                                                │
  │  Authorization を差し替え: Bearer <本物 access_token>  ← プロキシがインジェクト
  │  ChatGPT-Account-Id: <account_id>  ← プロキシが正値で上書き
  │  転送先: https://chatgpt.com/backend-api/codex/responses
  │ ─────────────────────────────────────────────▶│ (OpenAI)
  │                                                │
  │  (8時間後) POST /oauth/token?cdx=<token>      │
  │  {"grant_type":"refresh_token",               │
  │   "refresh_token":"","client_id":"app_..."}   │
  │  ◀─────────────────────────────────────────── │
  │                                                │
  │  プロキシがホストの refresh_token を注入し     │
  │  https://auth.openai.com/oauth/token に転送   │
  │  新 access_token → "cdx-<hex64>" に差し替えて返す
  │                    (refresh_token は除外)      │
  │ ───────────────────────────────────────────▶  │
```

> **セキュリティ**: `refresh_token` および本物の `access_token` はコンテナに渡されません。
> コンテナが保持するのは CODEX_TOKEN と同一のプレースホルダーのみで、プロキシがすべての送信リクエストの Authorization ヘッダーを本物の access_token で差し替えます。
> コンテナが侵害されてもプレースホルダーは OpenAI への直接アクセスに使えず、CODEX_TOKEN が失効すれば `/oauth/token` リフレッシュも不可能になります。

---

## API エンドポイント仕様

### `GET /token` — クレデンシャル取得

コンテナが短命トークンを使って認証情報を取得するエンドポイントです。

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

### `POST /oauth/token` — OAuthトークンリフレッシュ中継

Codex CLI が `CODEX_REFRESH_TOKEN_URL_OVERRIDE` 経由でトークンリフレッシュを要求するエンドポイントです。
プロキシは `refresh_token` をホストのものに差し替えて `https://auth.openai.com/oauth/token` に転送します。

**認証**: クエリパラメーター `?cdx=<短命トークン>` で行います（Codex CLI はリフレッシュリクエストにカスタムヘッダーを付けないため）。

**Codex CLI からのリクエスト形式**

Codex CLI は `application/json` で送信します（`client_id` は Codex CLI 側がハードコードして付加します）。

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
| `client_id` | そのまま通過 | Codex CLI が自身でハードコード済み。プロキシは関与しない |
| `grant_type` | そのまま通過 | 変更不要 |
| その他フィールド | そのまま通過 | 変更不要 |
| `refresh_token` (レスポンス) | **レスポンスから除外** | コンテナに新しい refresh_token を渡さない |
| `access_token` (レスポンス) | **プレースホルダーに差し替え** | 本物の新 access_token をコンテナに渡さない（プロキシ内部で更新） |
| `id_token` (レスポンス) | そのまま通過 | claims 抽出用（認証 credential ではない） |
| その他レスポンスフィールド | そのまま通過 | — |

**コンテナへのレスポンス**

OpenAI のレスポンスから `refresh_token` を除き、`access_token` をプレースホルダー（`cdx-<hex64>`）に差し替えて返します。
ホスト側の `access_token`・`id_token`・`refresh_token` はプロキシ内部で更新します（RFC 6749 §6 のトークンローテーションに対応）。

**エラーレスポンス**

| ステータス | 条件 |
|---|---|
| `400 Bad Request` | OAuth モードでない |
| `401 Unauthorized` | `cdx` パラメーターが欠如・無効・期限切れ |
| `405 Method Not Allowed` | POST 以外のメソッド |
| `502 Bad Gateway` | OpenAI への転送失敗 |

---

### `ANY /v1/*` — Responses API リバースプロキシ

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
version: 0.110.0
chatgpt-account-id: <account_id>   ← OAuth時: プロキシが正値で上書き
OpenAI-Organization: <org>         ← $OPENAI_ORGANIZATION があれば
```

---

### `ANY /chatgpt/*` — ChatGPT backend-api プロキシ

`/chatgpt/` 以下を `https://chatgpt.com/backend-api/` に転送します。
OAuth モードのコンテナは `~/.config/codex/config.toml` の `chatgpt_base_url=http://proxy/chatgpt/` 経由でレート制限情報・アカウント情報取得リクエストをここへ送ります。

---

### `GET /health` — ヘルスチェック

```json
HTTP/1.1 200 OK
Content-Type: application/json

{
  "status": "ok",
  "active_tokens": 3
}
```

---

### `POST /revoke` — トークン失効

```
POST /revoke?container=<コンテナ名> HTTP/1.1
```

| ステータス | 条件 |
|---|---|
| `200 OK` | 失効成功 |
| `400 Bad Request` | `container` パラメーターが欠如 |
| `405 Method Not Allowed` | POST 以外のメソッド |

---

## コンテナへの環境変数

コンテナには以下の環境変数が注入されます。

| 変数名 | 内容 | 使用後の処理 |
|---|---|---|
| `CODEX_AUTH_PROXY_URL` | `http://<proxy>:<PORT>` | `unset`（entrypoint.sh が削除） |
| `CODEX_TOKEN` | `cdx-<hex64>` — `/token` 取得用 | `unset`（entrypoint.sh が削除） |
| `OPENAI_BASE_URL` | `http://<proxy>:<PORT>/v1` | 常時有効（Codex CLI が参照） |
| `CODEX_REFRESH_TOKEN_URL_OVERRIDE` | `http://<proxy>:<PORT>/oauth/token?cdx=<token>` | OAuth モード時のみ設定 |

さらに OAuth モードでは `entrypoint.sh` が以下のファイルを生成します。

| ファイル | 内容 |
|---|---|
| `/home/codex/.codex/auth.json` | `access_token`：プレースホルダー（`cdx-<hex64>`）、`id_token`：本物、`refresh_token`：空文字 |
| `/home/codex/.config/codex/config.toml` | `chatgpt_base_url = "http://<proxy>:<PORT>/chatgpt/"` |

> コンテナの `auth.json` に含まれる `access_token` は本物ではありません。
> Codex CLI が API リクエストを送る際のダミー Bearer トークンとして機能し、プロキシが本物の access_token に差し替えます。

---

## トークンの仕組み

### トークン形式

```
cdx-<64桁の16進数>

例: cdx-a1b2c3d4e5f6789012345678901234567890abcdef1234567890abcdef1234
```

- `crypto/rand` を使って 32 バイトの乱数を生成
- 16 進エンコードして `cdx-` プレフィックスを付与
- 合計 68 文字

### トークンライフサイクル

```
                 IssueToken()
                      │
                      ▼
              ┌───────────────┐
              │ tokenRecord    │
              │ Token: "cdx-" │
              │ ContainerName │
              │ IssuedAt      │
              │ ExpiresAt     │  ← IssuedAt + TTL (デフォルト 3600 秒)
              └───────┬───────┘
                      │ メモリ上に保管
                      ▼
              ┌───────────────┐
              │ tokens map    │◀─── RevokeToken() で即時削除
              │ [name -> rec] │
              └───────┬───────┘
                      │
               30秒ごとにスキャン
                      │
                      ▼
              期限切れトークンを削除 (expireLoop)
```

| 設定 | デフォルト値 | 変更方法 |
|---|---|---|
| TTL | 3600 秒 (1 時間) | `--token-ttl <秒数>` または `config.toml` の `default_token_ttl` |

---

## クレデンシャルの優先順位（API キーモード）

```
1. OPENAI_API_KEY 環境変数
2. ~/.config/codex-dock/apikey  (codex-dock auth set で保存)
3. ~/.codex/auth.json
```

OAuth モードは `~/.codex/auth.json` に `refresh_token` または `auth_mode: "chatgpt"` がある場合に有効になります。

---

## セキュリティ考慮事項

### 実装済みの保護

| 保護 | 実装 | 詳細 |
|---|---|---|
| API キーの隔離 | ✅ | コンテナには `CODEX_TOKEN` と同じプレースホルダーのみ渡す。本物のキーはプロキシがインジェクト |
| access_token の隔離 | ✅ | OAuth モードでも本物の access_token はコンテナに渡らない。プレースホルダーをプロキシが差し替え |
| refresh_token の保護 | ✅ | コンテナに渡さない。リフレッシュは `/oauth/token` 中継で実現 |
| 短命トークン | ✅ | TTL 付き、コンテナ停止時に即時失効 |
| API トラフィックの中継 | ✅ | `/v1/` と `/chatgpt/` のリバースプロキシで外部 API への直接通信を排除 |
| クレデンシャルのログ出力禁止 | ✅ | 認証情報を stdout/stderr に出力しない |
| `auth.json` の bind mount 禁止 | ✅ | コンテナ内の auth.json は access_token がプレースホルダーの安全なコピー |

### 既知の問題・制限

| ID | 問題 | 影響度 | 詳細 |
|---|---|---|---|
| F-NET-04 | コンテナから Auth Proxy に到達できない場合あり | 高 | `127.0.0.1` ではコンテナから到達不可。`dock-net` の gateway アドレスを `ListenAddr` に指定する必要あり |
| NF-SEC-01 | 平文 HTTP 通信 | 高 | TLS または UNIX ソケットが未実装 |
| F-AUTH-06 | コンテナ ID による照合なし | 中 | トークンはコンテナ名と紐付けられているが、コンテナ ID との照合なし |

---

## 実装コード早見表

```go
// Auth Proxy の作成
proxy, _ := authproxy.NewProxy(authproxy.Config{
    TokenTTL:   3600,
    Verbose:    true,
    ListenAddr: "10.200.0.1:0", // dock-net gateway
})

// 起動
proxy.Start()

// トークン発行（コンテナ起動前に呼ぶ）
token, _ := proxy.IssueToken("my-container", 3600)

// エンドポイント確認
fmt.Println(proxy.Endpoint()) // "http://10.200.0.1:XXXXX"

// OAuth モード判定
if proxy.IsOAuthMode() {
    // CODEX_REFRESH_TOKEN_URL_OVERRIDE を設定する
}

// トークン失効（コンテナ停止時に呼ぶ）
proxy.RevokeToken("my-container")

// プロキシ停止（全トークンをクリア）
defer proxy.Stop()
```
