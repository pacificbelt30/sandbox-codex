# Using Auth Proxy Standalone — Codex Configuration Guide

> [日本語](../proxy-standalone.md) | **English**

This guide explains how to use only the Auth Proxy component of codex-dock to run Codex CLI directly (outside a Docker container).

---

## Who This Guide Is For

Use this guide if you want to:

- Run Codex CLI directly on the host (without Docker containers)
- Run Codex CLI in your own container or CI environment
- Protect your API keys or OAuth tokens without exposing them to processes
- Use only the Auth Proxy security feature without `codex-dock run`'s container management

**If you're using `codex-dock run` normally, you don't need this guide.**
→ See [Quick Start](getting-started.md) instead.

---

## Prerequisites

| Requirement | Verification |
|---|---|
| codex-dock installed | `codex-dock --version` |
| Codex CLI installed | `codex --version` |
| Credentials configured | `codex-dock auth show` |

### Usage Patterns (Important)

There are two common standalone Auth Proxy patterns.

| Pattern | Typical Use | Recommended Setup |
|---|---|---|
| A. Run `codex` directly on host | Local development | Run proxy on `localhost:18080` |
| B. Use proxy from `codex-dock run` / custom containers | Dockerized workloads | Attach proxy to `dock-net-proxy` and apply firewall rules |

> For pattern B, keep proxy container name as `codex-auth-proxy` so it matches the default `codex-dock run --proxy-container-url` (`http://codex-auth-proxy:18080`).

> This setup will be configurable with Docker Compose in a future update.

---

## Step 1: Start the Auth Proxy

### Option A: Run as a Docker Container (Recommended)

#### Executor: Host

```bash
# Build the Auth Proxy image (first time only)
codex-dock proxy build

# Start proxy on dedicated network (recommended)
codex-dock proxy run \
  --name codex-auth-proxy \
  --network dock-net-proxy \
  --admin-secret YOUR_SECRET \
  --port 18080
```

The management API is accessible from the host at `http://localhost:18080`.

If you use the proxy from `codex-dock run` or custom worker containers, also run:

#### Executor: Host (includes root-required command)

```bash
# Create worker network
codex-dock network create

# Allow worker -> proxy communication
sudo codex-dock firewall create --proxy-container-url http://codex-auth-proxy:18080
```

When running `firewall create`, if `dock-net` / `dock-net-proxy` are missing,
`codex-dock` shows a warning and prompts whether to create them (`Create <network> now? [y/N]:`).
Choosing `y` creates the required network and then continues firewall setup.

Validation command (order matters):

```bash
sudo iptables -S DOCKER-USER
# Expected: proxy allow rules come first,
#           -i dock-net0 -j CODEX-DOCK is the final rule
```

Example (conceptual order):

```text
ACCEPT ... -i dock-net-proxy0 -o dock-net0  -m conntrack --ctstate RELATED,ESTABLISHED
ACCEPT ... -i dock-net0       -o dock-net-proxy0 -p tcp --dport 18080
CODEX-DOCK ... -i dock-net0
```

> If `CODEX-DOCK` appears earlier, traffic may never reach proxy allow rules and connectivity can fail.

> **Security**: Without `--admin-secret`, the admin API has no authentication.
> Always set it to restrict who can issue tokens.

### Option B: Run as a Local Process

#### Executor: Host

```bash
codex-dock proxy serve --listen 127.0.0.1:18080 --admin-secret YOUR_SECRET
```

This works when running Codex CLI on the same host.

---

## Step 2: Issue a Short-Lived Token

Use the admin API to issue a token for your session.

```bash
PROXY_URL="http://localhost:18080"
ADMIN_SECRET="YOUR_SECRET"
SESSION_NAME="my-codex-session"

TOKEN=$(curl -sf -X POST "$PROXY_URL/admin/issue" \
  -H "Content-Type: application/json" \
  -H "X-Proxy-Admin-Secret: $ADMIN_SECRET" \
  -d "{\"container\": \"$SESSION_NAME\", \"ttl\": 3600}" \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['token'])")

echo "Token: $TOKEN"
# → Token: cdx-a1b2c3d4e5f6...
```

| Parameter | Description |
|---|---|
| `container` | Session identifier (any string) |
| `ttl` | Token TTL in seconds (defaults to proxy's `default_token_ttl` = 3600 s) |

---

## Step 3: Configure Codex CLI

Set the token and proxy URL for Codex CLI.

### API Key Mode

#### Executor: Host/container running Codex CLI

```bash
PROXY_URL="http://localhost:18080"

export OPENAI_API_KEY="$TOKEN"         # placeholder token (not the real key)
export OPENAI_BASE_URL="$PROXY_URL/v1" # route API requests through proxy

# Start Codex CLI
codex
```

**Environment variables explained:**

| Variable | Value | Description |
|---|---|---|
| `OPENAI_API_KEY` | `cdx-xxxx...` | Placeholder token. Proxy injects real API key on outbound requests |
| `OPENAI_BASE_URL` | `http://localhost:18080/v1` | Routes Codex CLI requests through the proxy |

### OAuth Mode (ChatGPT Subscription)

OAuth mode requires additional file configuration.

```bash
PROXY_URL="http://localhost:18080"

# Retrieve credentials from proxy using the token from Step 2
RESPONSE=$(curl -sf "$PROXY_URL/token" -H "X-Codex-Token: $TOKEN")

# Write auth.json for Codex CLI
mkdir -p ~/.codex
python3 -c "
import sys, json
d = json.loads('''$RESPONSE''')
out = {
    'auth_mode': 'chatgpt',
    'OPENAI_API_KEY': None,
    'tokens': {
        'id_token':      d.get('oauth_id_token', ''),
        'access_token':  d.get('oauth_access_token', ''),
        'refresh_token': '',
        'account_id':    d.get('oauth_account_id', ''),
    },
    'last_refresh': d.get('oauth_last_refresh', ''),
}
print(json.dumps(out, indent=2))
" > ~/.codex/auth.json
chmod 600 ~/.codex/auth.json

# Write Codex CLI config.toml to route ChatGPT backend-api through proxy
mkdir -p ~/.config/codex
echo "chatgpt_base_url = \"$PROXY_URL/chatgpt/\"" > ~/.config/codex/config.toml

# Route OAuth token refresh through proxy
export CODEX_REFRESH_TOKEN_URL_OVERRIDE="$PROXY_URL/oauth/token?cdx=$TOKEN"

# Start Codex CLI
codex
```

**Configuration explained:**

| Setting | Value | Description |
|---|---|---|
| `~/.codex/auth.json` `tokens.access_token` | `cdx-xxxx...` | Placeholder; real token injected by proxy |
| `~/.codex/auth.json` `tokens.refresh_token` | `""` (empty) | Proxy handles refresh; container never holds it |
| `~/.codex/auth.json` `tokens.id_token` | Real JWT | Codex CLI uses for claims extraction only (not auth credential) |
| `~/.config/codex/config.toml` `chatgpt_base_url` | `http://localhost:18080/chatgpt/` | Routes ChatGPT backend-api calls through proxy |
| `CODEX_REFRESH_TOKEN_URL_OVERRIDE` | `http://localhost:18080/oauth/token?cdx=<token>` | Routes token refresh through proxy |

> **Security**: The `access_token` in `auth.json` is not real.
> It acts as a dummy Bearer token for Codex CLI API requests, which the proxy replaces with the real access_token.
> The `refresh_token` is held only by the proxy and never sent to the process.

---

## Step 4: Verify the Setup

```bash
# Check health (active_tokens should be >= 1)
curl -s http://localhost:18080/health
# → {"active_tokens":1,"status":"ok"}

# Verify API requests are proxied correctly
curl -s "$PROXY_URL/v1/models" \
  -H "Authorization: Bearer $TOKEN" \
  | python3 -m json.tool | head -20
```

---

## Step 5: Revoke the Token When Done

```bash
curl -sf -X POST \
  "http://localhost:18080/admin/revoke?container=$SESSION_NAME" \
  -H "X-Proxy-Admin-Secret: $ADMIN_SECRET"
```

Tokens expire automatically after TTL, but it's good practice to revoke them explicitly when a session ends.

---

## Configuration Summary

### API Key Mode

| Setting | Value |
|---|---|
| `OPENAI_API_KEY` | `cdx-xxxx...` (placeholder token from Step 2) |
| `OPENAI_BASE_URL` | `http://localhost:18080/v1` |

### OAuth Mode

| Setting | Value |
|---|---|
| `~/.codex/auth.json` | `access_token`: placeholder, `refresh_token`: empty, `id_token`: real JWT |
| `~/.config/codex/config.toml` | `chatgpt_base_url = "http://localhost:18080/chatgpt/"` |
| `CODEX_REFRESH_TOKEN_URL_OVERRIDE` | `http://localhost:18080/oauth/token?cdx=<token>` |

---

## Check Operating Mode

```bash
curl -sf http://localhost:18080/admin/mode \
  -H "X-Proxy-Admin-Secret: $ADMIN_SECRET"
# → {"oauth_mode": false}   # API key mode
# → {"oauth_mode": true}    # OAuth mode
```

---

## Troubleshooting

### `401 Unauthorized` response

- Token may have expired → re-issue a token in Step 2
- Check that `$TOKEN` variable is set correctly

### `curl: Connection refused`

- Auth Proxy is not running → start it with `codex-dock proxy run` or `codex-dock proxy serve`
- Check that `PROXY_URL` port number is correct

### `codex-dock run` cannot reach `codex-auth-proxy:18080`

Check the following in order:

```bash
# 1) Verify proxy container network attachment
docker inspect codex-auth-proxy --format '{{json .NetworkSettings.Networks}}'

# 2) Verify bridge NIC names (usually dock-net-proxy0)
ip a | rg 'dock-net|proxy'

# 3) Verify actual DOCKER-USER rule order (prefer -S over -L)
sudo iptables -S DOCKER-USER
```

- If you changed `proxy run --name`, also update `run --proxy-container-url` to match.
- If `CODEX-DOCK` appears before proxy allow rules in `DOCKER-USER`, traffic may be dropped. Recreate rules with `sudo codex-dock firewall rm && sudo codex-dock firewall create`.

### API key mode expected but OAuth mode needed

- Run `codex-dock auth show` to verify the active auth mode
- If `~/.codex/auth.json` contains `refresh_token`, OAuth mode is activated

---

## Related Documentation

- [Auth Proxy Specification](auth-proxy.md) — Proxy architecture and flow diagrams
- [API Endpoint Reference](auth-proxy/endpoints.md) — Admin API specification
- [Token Lifecycle & Security](auth-proxy/tokens.md) — Token lifecycle details
- [`codex-dock proxy` command](commands/proxy.md) — How to start the proxy
- [Quick Start](getting-started.md) — Normal workflow using `codex-dock run`
