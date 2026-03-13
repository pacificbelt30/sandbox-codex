# Auth Proxy Specification

> [日本語](../auth-proxy.md) | **English**

The Auth Proxy is the core security component of codex-dock.
It provides authentication information safely to containers via short-lived tokens, without passing actual API keys or OAuth credentials.
It also proxies all OpenAI API traffic called by Codex CLI (Responses API, token refresh, ChatGPT backend-api), ensuring that **containers hold only placeholder tokens** and real credentials never reach them.

---

## Overview

```
┌──────────────────────────────────────────────────────────────────────┐
│  Host Environment                                                      │
│                                                                        │
│  ~/.codex/auth.json         ~/.config/codex-dock/apikey               │
│  (OAuth credentials)         (API key)                                 │
│          │                           │                                 │
│          └───────────┬───────────────┘                                 │
│                      ▼                                                 │
│  ┌──────────────────────────────────────────────────────────────────┐ │
│  │  Auth Proxy (<dock-net-gateway>:PORT)      ← random port         │ │
│  │                                                                  │ │
│  │  GET  /token        Token validation → credential delivery       │ │
│  │  GET  /health       Health check                                 │ │
│  │  POST /revoke       Token revocation                             │ │
│  │  POST /oauth/token  OAuth token refresh relay                    │ │
│  │  ANY  /v1/*         Responses API reverse proxy                  │ │
│  │  ANY  /chatgpt/*    ChatGPT backend-api proxy                    │ │
│  └──────────────────────────────────────────────────────────────────┘ │
│                      │ short-lived token (cdx-xxxx)                    │
│                      ▼                                                 │
│  ┌──────────────────────────────────────────────────────────────────┐ │
│  │  Sandbox Container                                               │ │
│  │  CODEX_TOKEN                                                     │ │
│  │  CODEX_AUTH_PROXY_URL                                            │ │
│  │  OPENAI_BASE_URL            ← points to /v1 proxy                │ │
│  │  CODEX_REFRESH_TOKEN_URL_OVERRIDE (OAuth mode only)              │ │
│  └──────────────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────────────┘
```

---

## Authentication Modes

### API Key Mode

Reads API key from the `OPENAI_API_KEY` environment variable or `~/.config/codex-dock/apikey` file.

```
Host (Auth Proxy)                          Container (entrypoint.sh)
  │                                                │
  │  Issue short-lived token                       │
  │  env: CODEX_TOKEN=cdx-<hex64>                 │
  │  env: OPENAI_BASE_URL=http://proxy/v1         │
  │ ───────────────────────────────────────────▶  │
  │                                                │ on startup
  │  GET /token                                    │
  │  X-Codex-Token: cdx-<hex64>                   │
  │  ◀─────────────────────────────────────────── │
  │                                                │
  │  200 OK {"api_key": "cdx-<hex64>"}  ← placeholder
  │ ───────────────────────────────────────────▶  │
  │                                                │
  │                                                │ export OPENAI_API_KEY=cdx-<hex64>  ← dummy
  │                                                │ unset CODEX_TOKEN
  │                                                │ exec codex
  │                                                │
  │  POST /v1/responses ← Codex CLI               │
  │  Authorization: Bearer cdx-<hex64>  ← dummy   │
  │  ◀─────────────────────────────────────────── │
  │                                                │
  │  Replace Authorization: Bearer sk-...         │ ← proxy injects real key
  │  Forward to: https://api.openai.com/v1/responses
  │ ─────────────────────────────────────────────▶│ (OpenAI)
```

### OAuth Mode (ChatGPT Subscription)

Automatically activates OAuth mode when `~/.codex/auth.json` contains a `refresh_token` field or `auth_mode: "chatgpt"` is set.

```
Host (Auth Proxy)                          Container (entrypoint.sh)
  │                                                │
  │  Read from ~/.codex/auth.json:                 │
  │   access_token, id_token,                     │
  │   refresh_token (host only), account_id       │
  │                                                │
  │  Issue short-lived token                       │
  │  env: CODEX_TOKEN=cdx-<hex64>                 │
  │  env: OPENAI_BASE_URL=http://proxy/v1         │
  │  env: CODEX_REFRESH_TOKEN_URL_OVERRIDE=       │
  │       http://proxy/oauth/token?cdx=<token>    │
  │ ───────────────────────────────────────────▶  │
  │                                                │ on startup
  │  GET /token                                    │
  │  X-Codex-Token: cdx-<hex64>                   │
  │  ◀─────────────────────────────────────────── │
  │                                                │
  │  200 OK                                        │
  │  {"oauth_access_token": "cdx-<hex64>",        │ ← placeholder (same as CODEX_TOKEN)
  │   "oauth_id_token":     "ey...",              │ ← real JWT (for claims extraction)
  │   "oauth_account_id":   "...",                │
  │   "oauth_last_refresh": "..."}                │
  │  ※ oauth_access_token is not real / no oauth_refresh_token
  │ ───────────────────────────────────────────▶  │
  │                                                │
  │                                                │ write /home/codex/.codex/auth.json
  │                                                │   access_token: "cdx-<hex64>"  ← dummy
  │                                                │   refresh_token: "" (empty)
  │                                                │ write /home/codex/.config/codex/config.toml
  │                                                │   chatgpt_base_url=http://proxy/chatgpt/
  │                                                │ unset CODEX_TOKEN
  │                                                │ exec codex
  │                                                │
  │  POST /v1/responses ← Codex CLI               │
  │  Authorization: Bearer cdx-<hex64>  ← dummy   │
  │  ChatGPT-Account-Id: <account_id>             │
  │  ◀─────────────────────────────────────────── │
  │                                                │
  │  Replace Authorization: Bearer <real access_token>  ← proxy injects
  │  Overwrite ChatGPT-Account-Id: <account_id>   ← proxy corrects value
  │  Forward to: https://chatgpt.com/backend-api/codex/responses
  │ ─────────────────────────────────────────────▶│ (OpenAI)
  │                                                │
  │  (after 8h) POST /oauth/token?cdx=<token>     │
  │  {"grant_type":"refresh_token",               │
  │   "refresh_token":"","client_id":"app_..."}   │
  │  ◀─────────────────────────────────────────── │
  │                                                │
  │  Proxy injects host refresh_token and         │
  │  forwards to https://auth.openai.com/oauth/token
  │  Replaces new access_token → "cdx-<hex64>" before returning
  │                    (refresh_token excluded)    │
  │ ───────────────────────────────────────────▶  │
```

> **Security**: Neither `refresh_token` nor the real `access_token` reaches the container.
> The container holds only the placeholder (same as CODEX_TOKEN), which cannot access OpenAI directly.
> Once `CODEX_TOKEN` expires, `/oauth/token` refresh becomes impossible as well.

---

## API Endpoint Reference

### `GET /token` — Credential Retrieval

The endpoint through which containers retrieve authentication information using their short-lived token.

**Request**

```
GET /token HTTP/1.1
X-Codex-Token: cdx-<64 hex digits>
```

**Response (API Key Mode)**

```json
HTTP/1.1 200 OK
Content-Type: application/json

{
  "api_key": "cdx-a1b2c3d4...",
  "container_name": "codex-brave-atlas"
}
```

> `api_key` is not the real API key — it is the same placeholder value as `CODEX_TOKEN`.
> The proxy injects the real API key into the `Authorization` header on outbound requests.

**Response (OAuth Mode)**

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

> - `oauth_access_token` is a placeholder, not the real access token. The proxy injects the real value on outbound requests.
> - `oauth_id_token` is the real JWT, needed by Codex CLI to read claims like `chatgpt_account_id` locally (no signature verification).
> - `oauth_refresh_token` is never returned (by security design).

**Error Responses**

| Status | Condition |
|---|---|
| `401 Unauthorized` | `X-Codex-Token` header missing, invalid, or expired |
| `405 Method Not Allowed` | Non-GET method |

---

### `POST /oauth/token` — OAuth Token Refresh Relay

The endpoint Codex CLI uses for token refresh via `CODEX_REFRESH_TOKEN_URL_OVERRIDE`.
The proxy replaces `refresh_token` with the host's value and forwards to `https://auth.openai.com/oauth/token`.

**Authentication**: via query parameter `?cdx=<short-lived token>` (Codex CLI does not add custom headers to refresh requests).

**Request format from Codex CLI**

Codex CLI sends `application/json` (with `client_id` hardcoded by Codex CLI itself):

```
POST /oauth/token?cdx=<cdx-xxxx> HTTP/1.1
Content-Type: application/json

{
  "client_id":    "app_EMoamEEZ73f0CkXaXp7hrann",
  "grant_type":   "refresh_token",
  "refresh_token": ""
}
```

**Format sent by proxy to OpenAI**

Only the `refresh_token` field is replaced with the host's real value; all other fields are passed through:

```
POST https://auth.openai.com/oauth/token HTTP/1.1
Content-Type: application/json

{
  "client_id":    "app_EMoamEEZ73f0CkXaXp7hrann",
  "grant_type":   "refresh_token",
  "refresh_token": "<host's real refresh_token>"
}
```

**Field Processing Summary**

| Field | Processing | Reason |
|---|---|---|
| `refresh_token` (request) | **Replaced with host value** | Container's `refresh_token` is empty |
| `client_id` | Passed through | Hardcoded by Codex CLI; proxy does not touch it |
| `grant_type` | Passed through | No modification needed |
| Other request fields | Passed through | No modification needed |
| `refresh_token` (response) | **Excluded from response** | Do not pass new refresh_token to container |
| `access_token` (response) | **Replaced with placeholder** | Do not pass real new access_token to container |
| `id_token` (response) | Passed through | For claims extraction (not an auth credential) |
| Other response fields | Passed through | — |

**Response to Container**

Returns the OpenAI response with `refresh_token` removed and `access_token` replaced with the placeholder (`cdx-<hex64>`).
The host's `access_token`, `id_token`, and `refresh_token` are updated internally (handles RFC 6749 §6 token rotation).

**Error Responses**

| Status | Condition |
|---|---|
| `400 Bad Request` | Not in OAuth mode |
| `401 Unauthorized` | `cdx` parameter missing, invalid, or expired |
| `405 Method Not Allowed` | Non-POST method |
| `502 Bad Gateway` | Forwarding to OpenAI failed |

---

### `ANY /v1/*` — Responses API Reverse Proxy

Codex CLI points `OPENAI_BASE_URL=http://proxy/v1` and sends all Responses API requests through the proxy.

| Auth Mode | Forwarded To |
|---|---|
| API Key | `https://api.openai.com/v1/<path>` |
| OAuth / ChatGPT | `https://chatgpt.com/backend-api/codex/<path>` |

- The `Authorization` header's placeholder value is **overwritten with the host's real credentials**
- In OAuth mode, `ChatGPT-Account-Id` header is also overwritten with the correct `oauthCreds.AccountID`
- Hop-by-hop headers (`Connection`, `Transfer-Encoding`, etc.) are stripped
- Response status, headers, and body are returned as-is to the container
- WebSocket upgrade requests are tunneled with `Authorization` and `ChatGPT-Account-Id` replaced

**Actual headers sent upstream (after proxy substitution)**

```
Authorization: Bearer <real access_token or api_key>  ← proxy injects
Content-Type: application/json
version: 0.110.0
chatgpt-account-id: <account_id>   ← OAuth mode: proxy overwrites with correct value
OpenAI-Organization: <org>         ← if $OPENAI_ORGANIZATION is set
```

---

### `ANY /chatgpt/*` — ChatGPT Backend-API Proxy

Forwards `/chatgpt/` paths to `https://chatgpt.com/backend-api/`.
In OAuth mode, containers use `~/.config/codex/config.toml`'s `chatgpt_base_url=http://proxy/chatgpt/` for rate limit and account info requests.

---

### `GET /health` — Health Check

```json
HTTP/1.1 200 OK
Content-Type: application/json

{
  "status": "ok",
  "active_tokens": 3
}
```

---

### `POST /revoke` — Token Revocation

```
POST /revoke?container=<container-name> HTTP/1.1
```

| Status | Condition |
|---|---|
| `200 OK` | Revocation successful |
| `400 Bad Request` | `container` parameter missing |
| `405 Method Not Allowed` | Non-POST method |

---

## Container Environment Variables

The following environment variables are injected into containers:

| Variable | Content | Post-use Handling |
|---|---|---|
| `CODEX_AUTH_PROXY_URL` | `http://<proxy>:<PORT>` | `unset` (removed by entrypoint.sh) |
| `CODEX_TOKEN` | `cdx-<hex64>` — used to call `/token` | `unset` (removed by entrypoint.sh) |
| `OPENAI_BASE_URL` | `http://<proxy>:<PORT>/v1` | Always active (referenced by Codex CLI) |
| `CODEX_REFRESH_TOKEN_URL_OVERRIDE` | `http://<proxy>:<PORT>/oauth/token?cdx=<token>` | OAuth mode only |

In OAuth mode, `entrypoint.sh` also generates the following files:

| File | Content |
|---|---|
| `/home/codex/.codex/auth.json` | `access_token`: placeholder (`cdx-<hex64>`), `id_token`: real, `refresh_token`: empty string |
| `/home/codex/.config/codex/config.toml` | `chatgpt_base_url = "http://<proxy>:<PORT>/chatgpt/"` |

> The `access_token` in the container's `auth.json` is not real.
> It serves as a dummy Bearer token for Codex CLI API requests, which the proxy replaces with the real access_token.

---

## Token Mechanics

### Token Format

```
cdx-<64 hex digits>

Example: cdx-a1b2c3d4e5f6789012345678901234567890abcdef1234567890abcdef1234
```

- 32 bytes of random data generated using `crypto/rand`
- Hex-encoded with `cdx-` prefix
- Total 68 characters

### Token Lifecycle

```
                 IssueToken()
                      │
                      ▼
              ┌───────────────┐
              │ tokenRecord    │
              │ Token: "cdx-" │
              │ ContainerName │
              │ IssuedAt      │
              │ ExpiresAt     │  ← IssuedAt + TTL (default 3600s)
              └───────┬───────┘
                      │ stored in memory
                      ▼
              ┌───────────────┐
              │ tokens map    │◀─── RevokeToken() removes immediately
              │ [name -> rec] │
              └───────┬───────┘
                      │
               scan every 30s
                      │
                      ▼
              delete expired tokens (expireLoop)
```

| Setting | Default | How to Change |
|---|---|---|
| TTL | 3600s (1 hour) | `--token-ttl <seconds>` or `config.toml`'s `default_token_ttl` |

---

## Credential Priority (API Key Mode)

```
1. OPENAI_API_KEY environment variable
2. ~/.config/codex-dock/apikey  (saved by codex-dock auth set)
3. ~/.codex/auth.json
```

OAuth mode is activated when `~/.codex/auth.json` contains `refresh_token` or `auth_mode: "chatgpt"`.

---

## Security Considerations

### Implemented Protections

| Protection | Implemented | Details |
|---|---|---|
| API key isolation | ✅ | Container receives only placeholder (same as CODEX_TOKEN); proxy injects real key |
| access_token isolation | ✅ | Real access_token never reaches container even in OAuth mode; proxy replaces placeholder |
| refresh_token protection | ✅ | Never passed to container; refresh handled via `/oauth/token` relay |
| Short-lived tokens | ✅ | TTL-bound; immediately revoked on container stop |
| API traffic relay | ✅ | Reverse proxy on `/v1/` and `/chatgpt/` eliminates direct external API access from containers |
| No credential logging | ✅ | Auth information never written to stdout/stderr |
| No auth.json bind mount | ✅ | Container's auth.json contains a safe copy with placeholder access_token |

### Known Issues / Limitations

| ID | Issue | Severity | Details |
|---|---|---|---|
| F-NET-04 | Container may not reach Auth Proxy | High | `127.0.0.1` is unreachable from containers; must bind to dock-net gateway address |
| NF-SEC-01 | Plain HTTP communication | High | TLS or UNIX socket not yet implemented |
| F-AUTH-06 | No container ID verification | Medium | Token is tied to container name but not verified against container ID |

---

## Implementation Quick Reference

```go
// Create Auth Proxy
proxy, _ := authproxy.NewProxy(authproxy.Config{
    TokenTTL:   3600,
    Verbose:    true,
    ListenAddr: "10.200.0.1:0", // dock-net gateway
})

// Start
proxy.Start()

// Issue token (call before container launch)
token, _ := proxy.IssueToken("my-container", 3600)

// Check endpoint
fmt.Println(proxy.Endpoint()) // "http://10.200.0.1:XXXXX"

// Check OAuth mode
if proxy.IsOAuthMode() {
    // set CODEX_REFRESH_TOKEN_URL_OVERRIDE
}

// Revoke token (call on container stop)
proxy.RevokeToken("my-container")

// Stop proxy (clears all tokens)
defer proxy.Stop()
```
