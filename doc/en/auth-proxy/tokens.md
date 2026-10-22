# Short-lived tokens

[日本語](../../auth-proxy/tokens.md) | **English**

- A `cdx-...` value is a bearer token for the proxy, not a real provider API key.
- Anyone holding a valid token can use the AI proxy routes until it expires or is revoked.

## Format and lifetime

- Tokens use 32 cryptographically random bytes, encoded as 64 hexadecimal characters after `cdx-` (68 characters total).
- The default TTL is 3600 seconds.

```bash
oaiso run --agent codex --token-ttl 7200
```

- Expired tokens are rejected immediately and their active HTTP/WS connections are canceled.
- The 30-second record cleanup loop does not delay authentication expiry.

## Lifecycle

| Action | Result |
|---|---|
| Create worker | Issue and activate before releasing user code |
| Stop | Revoke token and cancel active connections |
| Restart | Restore the saved placeholder and release code after Kata checks |
| Reissue or restore for the same name | Cancel the previous lease and connections |
| Remove | Revoke and reclaim resources; retain retry records on failure |
| Stop proxy | Revoke all tokens and connections; issuance records are in memory |

- Use `oaiso start NAME` to restart a stopped worker.
- The admin restore API accepts saved placeholders, not real API keys or another worker's token.

## Worker data

- `CODEX_TOKEN` and `CODEX_AUTH_PROXY_URL` bootstrap authentication and are unset by the entrypoint.
- Client files still retain placeholders.
- API-key variables contain placeholders.
- OAuth auth.json has a placeholder access token, an empty refresh token and a fresh dummy ID JWT containing only account ID, plan and the FedRAMP flag.
- Real ID tokens remain on the host.
- Codex configuration points requests to the proxy.
- `CODEX_REFRESH_TOKEN_URL_OVERRIDE` includes the worker token.
- Paths depend on the selected user's HOME, not a fixed `/home/codex`.

## Limits

- All HTTP/WS AI routes validate issued, unexpired tokens and reject duplicate or conflicting authentication.
- There is no source container-ID attestation.
- Worker-to-proxy traffic is plain HTTP on dedicated networks.
- Choose a TTL suitable for the task and revoke unused tokens by stopping or removing workers.

See [API reference](endpoints.md) and [security](../security.md).
