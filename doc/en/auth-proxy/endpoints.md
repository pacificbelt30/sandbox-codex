# Auth API

[日本語](../../auth-proxy/endpoints.md) | **English**

See [overview](../auth-proxy.md) and [tokens](tokens.md).
These routes belong to the auth role. The egress role provides forwarding and health only.

## Listeners

| Listener | Default URL | Caller |
|---|---|---|
| Data | `http://codex-auth-proxy:18080` | Workers |
| Admin | `http://127.0.0.1:18081` | Host CLI |

`proxy run` separates admin from data. For a custom `proxy serve`, specify `--admin-listen`.
If an admin secret is configured, `/admin/*` requires `X-Proxy-Admin-Secret`.

## Routes

| Method | Path | Authentication / purpose |
|---|---|---|
| GET | `/token` | `X-Codex-Token`; obtain placeholders |
| POST | `/oauth/token` | `?cdx=<worker token>`; OpenAI refresh |
| ANY / WS | `/v1/*`, `/chatgpt/*` | Bearer worker token |
| ANY / WS | `/anthropic/*` | `x-api-key` or Bearer worker token |
| GET | `/health` | No authentication; process health |
| POST | `/revoke` | Legacy data-plane revocation; prefer `/admin/revoke` |
| POST | `/admin/issue` | Issue token |
| POST | `/admin/restore` | Restore saved placeholder |
| POST | `/admin/revoke` | Revoke token |
| GET | `/admin/mode` | Authentication modes and capability |

## `GET /token`

Send `X-Codex-Token: cdx-<64 hexadecimal digits>`.
Returns 200 JSON in API-key mode:

```json
{"api_key": "cdx-<64 hexadecimal digits>", "container_name": "worker-name"}
```

OAuth mode returns:

```json
{
  "oauth_access_token": "cdx-<64 hexadecimal digits>",
  "oauth_id_token": "<dummy JWT containing selected account metadata>",
  "oauth_account_id": "<account ID>",
  "oauth_last_refresh": "<timestamp>",
  "container_name": "worker-name"
}
```

The API/access token is the submitted placeholder. Real access/refresh/ID tokens are not returned.
The ID field is a fresh dummy JWT containing only account ID, plan and the FedRAMP flag, without the original signature or arbitrary claims. Successful responses set `Cache-Control: no-store`.
Invalid, missing or expired tokens return 401; wrong methods return 405.

## `POST /oauth/token`

OpenAI refresh relay used by `CODEX_REFRESH_TOKEN_URL_OVERRIDE`.
Authenticate with `?cdx=<worker token>` because the client does not attach custom refresh headers.

```json
{"client_id": "<client ID>", "grant_type": "refresh_token", "refresh_token": ""}
```

The proxy substitutes the real host refresh token before forwarding to OpenAI.
Success responses contain only `access_token` (the worker placeholder) and `id_token` (fresh dummy JWT). Refresh tokens and arbitrary issuer fields are not returned. Real updated credentials are persisted on the proxy side.
Upstream error status is preserved; the body becomes `{"error":"<code>"}`. Only `refresh_token_expired`, `refresh_token_reused`, `refresh_token_invalidated` and `invalid_grant` are retained in canonical lowercase; other codes become `oauth_refresh_failed`. Issuer messages and reflected credentials are never forwarded. Success and upstream-error responses set `Cache-Control: no-store`.

Errors: 400 for wrong mode or malformed request, 401 for invalid worker token, 405 for wrong method, 502 for upstream connection failure.

## AI HTTP and WebSocket routes

`/v1/*` and `/chatgpt/*` require `Authorization: Bearer <worker token>`.
`/anthropic/*` accepts `x-api-key` or Bearer. If both are present they must match.
Missing, unissued, expired, revoked, duplicate or conflicting credentials return 401 before an upstream connection.
Expiry, revocation and replacement cancel existing HTTP streams and WS tunnels.

| Route / mode | Upstream |
|---|---|
| `/v1/*`, API key | `https://api.openai.com/v1/*` |
| `/v1/*`, ChatGPT OAuth | `https://chatgpt.com/backend-api/codex/*` |
| `/chatgpt/*` | `https://chatgpt.com/backend-api/*` |
| `/anthropic/*` | `https://api.anthropic.com/*` |

Worker authentication is removed and real provider credentials are injected.
ChatGPT account ID is overwritten with the correct value.
HTTP hop-by-hop headers are removed; upstream status/body and relevant headers are returned.
SSE is streamed and validated WebSockets are tunneled.

## `GET /health`

```json
{"status": "ok", "active_tokens": 3}
```

The count is in-memory records, which can include expired records awaiting cleanup.
This is not a proof of token validity or upstream AI connectivity.

## Admin API

If configured, supply `X-Proxy-Admin-Secret: <secret>` on all admin calls.
Wrong secrets return 401 and wrong methods return 405.

### `POST /admin/issue`

```json
{"container": "worker-name", "ttl": 3600}
```

`container` is required. An omitted or nonpositive TTL uses the proxy default.
Returns 200 with `{"token": "cdx-..."}`. Invalid JSON or an empty name returns 400.
Issuing for the same name cancels the previous lease and connections.

### `POST /admin/restore`

```json
{"container": "worker-name", "token": "cdx-<64 hexadecimal digits>", "ttl": 3600}
```

Reactivates a saved placeholder; success is 204.
Invalid name/format/TTL or a token assigned to another worker returns 400.
Real API keys are not accepted. Managed restart handles this automatically.

### `POST /admin/revoke`

Specify `?container=worker-name`. Success is 200; missing name is 400.
Cancels the lease and its connections.
Legacy `POST /revoke` uses the same query but does not validate an admin secret; use the separated admin route for management tools.

### `GET /admin/mode`

```json
{
  "oauth_mode": true,
  "anthropic_available": false,
  "anthropic_oauth_mode": false,
  "worker_token_auth": true
}
```

Current clients reject proxies without `worker_token_auth: true`.
Rebuild and recreate old proxies before creating workers. See [operations](../../operations.md#cliプロキシを更新する) (Japanese).
