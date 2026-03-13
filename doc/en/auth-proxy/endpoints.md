# Auth Proxy — API Endpoint Reference

> [日本語](../../../auth-proxy/endpoints.md) | **English**

Full HTTP endpoint specifications for the Auth Proxy.

- [Overview & Deployment](../auth-proxy.md)
- **API Endpoint Reference** ← this page
- [Token Lifecycle & Security](tokens.md)

---

## Endpoint Summary

| Endpoint | Method | Purpose |
|---|---|---|
| [`/token`](#get-token--credential-retrieval) | GET | Container retrieves credentials |
| [`/oauth/token`](#post-oauthtoken--oauth-token-refresh-relay) | POST | OAuth token refresh relay |
| [`/v1/*`](#any-v1--responses-api-reverse-proxy) | ANY | Responses API reverse proxy |
| [`/chatgpt/*`](#any-chatgpt--chatgpt-backend-api-proxy) | ANY | ChatGPT backend-api proxy |
| [`/health`](#get-health--health-check) | GET | Health check |
| [`/revoke`](#post-revoke--token-revocation) | POST | Token revocation |
| [`/admin/issue`](#post-adminissue--token-issuance-admin) | POST | Issue token (admin) |
| [`/admin/revoke`](#post-adminrevoke--token-revocation-admin) | POST | Revoke token (admin) |
| [`/admin/mode`](#get-adminmode--mode-check) | GET | Check operating mode |

---

## `GET /token` — Credential Retrieval

Called by `entrypoint.sh` at container startup to exchange the short-lived token for credentials.

**Request**

```
GET /token HTTP/1.1
X-Codex-Token: cdx-<64 hex chars>
```

**Response (API key mode)**

```json
HTTP/1.1 200 OK
Content-Type: application/json

{
  "api_key": "cdx-a1b2c3d4...",
  "container_name": "codex-brave-atlas"
}
```

> `api_key` is **not** the real API key — it is the same placeholder as `CODEX_TOKEN`.
> The proxy injects the real key into `Authorization` on every outbound request.

**Response (OAuth mode)**

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

> - `oauth_access_token` is a placeholder (same as `CODEX_TOKEN`). The real access token is injected by the proxy.
> - `oauth_id_token` is the real JWT. Codex CLI uses it locally to extract `chatgpt_account_id` and `chatgpt_plan_type` claims (no signature verification).
> - `oauth_refresh_token` is intentionally omitted (security design).

**Error Responses**

| Status | Condition |
|---|---|
| `401 Unauthorized` | Missing, invalid, or expired `X-Codex-Token` header |
| `405 Method Not Allowed` | Non-GET method |

---

## `POST /oauth/token` — OAuth Token Refresh Relay

Codex CLI calls this endpoint via `CODEX_REFRESH_TOKEN_URL_OVERRIDE` to refresh its OAuth token.
The proxy substitutes the host's real `refresh_token` and forwards to `https://auth.openai.com/oauth/token`.

**Authentication**: Query parameter `?cdx=<short-lived-token>` (Codex CLI does not add custom headers on refresh requests).

**Request from Codex CLI**

```
POST /oauth/token?cdx=<cdx-xxxx> HTTP/1.1
Content-Type: application/json

{
  "client_id":    "app_EMoamEEZ73f0CkXaXp7hrann",
  "grant_type":   "refresh_token",
  "refresh_token": ""
}
```

**Field Processing**

| Field | Action | Reason |
|---|---|---|
| `refresh_token` (request) | **Replaced with host value** | Container's `refresh_token` is empty |
| `client_id` | Passed through | Hardcoded by Codex CLI |
| `grant_type` | Passed through | No change needed |
| `refresh_token` (response) | **Stripped** | Prevents container from receiving new refresh token |
| `access_token` (response) | **Replaced with placeholder** | Real access token never reaches container |
| `id_token` (response) | Passed through | For claims extraction only |

**Error Responses**

| Status | Condition |
|---|---|
| `400 Bad Request` | Not in OAuth mode |
| `401 Unauthorized` | Missing, invalid, or expired `cdx` parameter |
| `405 Method Not Allowed` | Non-POST method |
| `502 Bad Gateway` | Failed to reach OpenAI |

---

## `ANY /v1/*` — Responses API Reverse Proxy

Containers set `OPENAI_BASE_URL=http://proxy/v1` so all Responses API requests flow through the proxy.

| Auth Mode | Upstream |
|---|---|
| API key | `https://api.openai.com/v1/<path>` |
| OAuth / ChatGPT | `https://chatgpt.com/backend-api/codex/<path>` |

- `Authorization` header is **overwritten** with the real host credentials
- In OAuth mode, `ChatGPT-Account-Id` is also overwritten with the correct `AccountID`
- Hop-by-hop headers (`Connection`, `Transfer-Encoding`, etc.) are stripped
- WebSocket upgrades are tunneled with the same credential injection

---

## `ANY /chatgpt/*` — ChatGPT backend-api Proxy

Forwards `/chatgpt/` to `https://chatgpt.com/backend-api/`.
OAuth-mode containers set `chatgpt_base_url=http://proxy/chatgpt/` in Codex CLI config to route rate-limit and account-info requests here.

---

## `GET /health` — Health Check

```json
HTTP/1.1 200 OK
Content-Type: application/json

{
  "status": "ok",
  "active_tokens": 3
}
```

---

## `POST /revoke` — Token Revocation

Called by `sandbox.Manager.Stop()` when a container is stopped.

```
POST /revoke?container=<container-name> HTTP/1.1
```

| Status | Condition |
|---|---|
| `200 OK` | Token revoked |
| `400 Bad Request` | Missing `container` parameter |
| `405 Method Not Allowed` | Non-POST method |

---

## Admin Endpoints (`/admin/*`)

Admin endpoints require the `X-Proxy-Admin-Secret` header (when `--admin-secret` is configured).

### `POST /admin/issue` — Token Issuance (Admin)

Issues a short-lived token for a named session.
Used when running Codex CLI without `codex-dock run`.
→ See [Using Auth Proxy Standalone](../proxy-standalone.md).

**Request**

```
POST /admin/issue HTTP/1.1
Content-Type: application/json
X-Proxy-Admin-Secret: <secret>

{
  "container": "my-session",
  "ttl": 3600
}
```

**Response**

```json
HTTP/1.1 200 OK
Content-Type: application/json

{"token": "cdx-a1b2c3d4..."}
```

---

### `POST /admin/revoke` — Token Revocation (Admin)

```
POST /admin/revoke?container=<name> HTTP/1.1
X-Proxy-Admin-Secret: <secret>
```

---

### `GET /admin/mode` — Mode Check

```json
HTTP/1.1 200 OK
Content-Type: application/json

{"oauth_mode": false}
```

---

## Related Documentation

- [Auth Proxy Overview & Deployment](../auth-proxy.md)
- [Token Lifecycle & Security](tokens.md)
- [Using Auth Proxy Standalone](../proxy-standalone.md)
- [Network Specification](../network.md)
