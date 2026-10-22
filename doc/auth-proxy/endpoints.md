# 認証API

**日本語** | [English](../en/auth-proxy/endpoints.md)

[認証プロキシの概要](../auth-proxy.md)と[短命トークン](tokens.md)も参照してください。
以下はauthロールのAPIです。egressロールは一般通信とhealthだけを提供します。

## 接続先

| リスナー | 既定URL | 接続元 |
|---|---|---|
| data | `http://codex-auth-proxy:18080` | ワーカー |
| admin | `http://127.0.0.1:18081` | ホストCLI |

`proxy run` は管理APIを別リスナーへ分離します。
`proxy serve` を独自配置する場合は、`--admin-listen` も明示してください。
管理secretを設定した場合、管理APIは `X-Proxy-Admin-Secret` ヘッダーを要求します。

## API一覧

| メソッド | パス | 用途・認証 |
|---|---|---|
| GET | `/token` | placeholder取得。`X-Codex-Token` |
| POST | `/oauth/token` | OpenAI OAuth更新。`?cdx=<短命トークン>` |
| ANY / WS | `/v1/*` | OpenAI API。Bearer |
| ANY / WS | `/chatgpt/*` | ChatGPT backend API。Bearer |
| ANY / WS | `/anthropic/*` | Anthropic API。`x-api-key` またはBearer |
| GET | `/health` | プロセスのhealth。認証不要 |
| POST | `/revoke` | 旧互換の失効API。通常の管理には `/admin/revoke` を使う |
| POST | `/admin/issue` | トークン発行 |
| POST | `/admin/restore` | 保存済みplaceholderの復元 |
| POST | `/admin/revoke` | トークン失効 |
| GET | `/admin/mode` | 認証モードと対応能力 |

独立profileでは、管理用 `/admin/mode` の `independent_profile` がtrueで、`X-Cxd-Auth-Profile` と `X-Cxd-Connection-Version` ヘッダーに名前と接続世代を返します。`case_proxy` は外部ケース用であり、ローカルprofileではfalseです。この情報に実資格情報は含みません。profileは管理secret32byte以上・別admin listenerを必須とします。

## `GET /token`

```http
GET /token HTTP/1.1
X-Codex-Token: cdx-<64桁の16進数>
```

200のJSON本文を返します。APIキーモードの例です。

```json
{"api_key": "cdx-<64桁の16進数>", "container_name": "worker-name"}
```

OAuthモードでは次の情報を返します。

```json
{
  "oauth_access_token": "cdx-<64桁の16進数>",
  "oauth_id_token": "<dummy JWT containing selected account metadata>",
  "oauth_account_id": "<account ID>",
  "oauth_last_refresh": "<timestamp>",
  "container_name": "worker-name"
}
```

`api_key` と `oauth_access_token` は提示した短命トークンと同じplaceholderです。
実access token・refresh token・ID tokenは返しません。
ID欄はaccount ID・plan・FedRAMPフラグだけを持つ新規ダミーJWTです。元の署名・任意claimは含みません。
正常応答には`Cache-Control: no-store`を設定します。
無効・欠落・期限切れのトークンは401、GET以外は405です。

## `POST /oauth/token`

`CODEX_REFRESH_TOKEN_URL_OVERRIDE` から使うOpenAI OAuth更新の中継です。
クライアントがカスタム認証ヘッダーを付けないため、短命トークンは `?cdx=...` に指定します。

```json
{
  "client_id": "<client ID>",
  "grant_type": "refresh_token",
  "refresh_token": ""
}
```

リクエストのrefresh tokenをホストの本物に差し替え、OpenAIのOAuth endpointへ送ります。
成功応答は`access_token`（placeholder）と`id_token`（新規ダミーJWT）の2項目だけです。
実refresh tokenや、上流が追加した任意フィールドは返しません。更新後の本物の認証情報はプロキシ側で保存します。
上流のエラーstatusは保持し、本文は`{"error":"<code>"}`へ置き換えます。`refresh_token_expired`・`refresh_token_reused`・`refresh_token_invalidated`・`invalid_grant`だけを正規化して保持し、それ以外は`oauth_refresh_failed`です。エラー本文に反射された資格情報や任意の説明文は返しません。成功・上流エラー応答は`Cache-Control: no-store`とします。

| ステータス | 条件 |
|---|---|
| 400 | OAuthモードではない、または不正なリクエスト |
| 401 | 短命トークンが無効・欠落・期限切れ |
| 405 | POST以外 |
| 502 | 上流への転送失敗 |

## AI APIのHTTP・WebSocket

`/v1/*` と `/chatgpt/*` は `Authorization: Bearer <短命トークン>` を要求します。
`/anthropic/*` は `x-api-key` またはBearerで受け付けます。
両方を指定する場合は同じ値が必要です。

未発行・失効・期限切れ・重複・矛盾する認証を、上流へ接続する前に401で拒否します。
HTTPとWebSocket upgradeに同じ検証を行います。
失効・期限切れ・置換時には、そのトークンの既存stream/tunnelも終了します。

| 経路・認証 | 上流 |
|---|---|
| `/v1/*`、OpenAI APIキー | `https://api.openai.com/v1/*` |
| `/v1/*`、ChatGPT OAuth | `https://chatgpt.com/backend-api/codex/*` |
| `/chatgpt/*` | `https://chatgpt.com/backend-api/*` |
| `/anthropic/*` | `https://api.anthropic.com/*` |

ワーカー用の認証ヘッダーを除去し、プロバイダーに対応する実認証を注入します。
ChatGPTではaccount IDも正しい値に上書きします。
HTTPのhop-by-hopヘッダーを除去し、上流のstatus・bodyと必要なヘッダーを返します。
SSEは逐次転送し、WebSocketは検証済み接続をトンネルします。

## `GET /health`

```json
{"status": "ok", "active_tokens": 3}
```

プロキシプロセスの応答とメモリ上の登録数です。
期限切れレコードの整理には間隔があるため、有効トークン数の厳密な保証や上流AIの疎通確認には使えません。

## 管理API

管理secretを設定した場合は、すべての `/admin/*` に次のヘッダーを付けます。

```http
X-Proxy-Admin-Secret: <管理secret>
```

不一致は401、指定メソッド以外は405です。

### `POST /admin/issue`

```json
{"container": "worker-name", "ttl": 3600}
```

`container` は必須です。`ttl` の省略または0以下はプロキシの既定値を使います。
成功は200で `{"token": "cdx-..."}` を返します。不正なJSONや空の名前は400です。
同じ名前へ再発行すると、前のリースとその接続を取り消します。

### `POST /admin/restore`

```json
{"container": "worker-name", "token": "cdx-<64桁の16進数>", "ttl": 3600}
```

停止ワーカーの保存済みplaceholderを再有効化します。成功は204です。
不正な名前・形式・TTL、別ワーカーに割り当て済みのトークンは400です。
実APIキーは受け付けません。通常は `oaiso vm start` が行います。

### `POST /admin/revoke`

`?container=worker-name` で対象を指定します。
成功は200、名前の欠落は400です。対象のリースと接続を取り消します。
旧data APIの `POST /revoke` も同じquery形式ですが、管理secretの検証はありません。
外部管理ツールは分離した `/admin/revoke` を使ってください。

### `GET /admin/mode`

```json
{
  "oauth_mode": true,
  "anthropic_available": false,
  "anthropic_oauth_mode": false,
  "worker_token_auth": true
}
```

`worker_token_auth` はワーカーのAI経路認証を実装していることを示します。
現行CLIはtrueでない旧プロキシを拒否します。
[プロキシを更新・再作成](../operations.md#cliプロキシを更新する)してからワーカーを作成してください。
