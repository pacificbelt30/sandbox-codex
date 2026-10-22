# Standalone proxy

[日本語](../proxy-standalone.md) | **English**

- This developer guide runs a local client without `oaiso run`.
- It does not create Kata isolation or worker networks.
- For normal use, follow the [quick start](getting-started.md).

## 1. Start a local proxy

- Prepare host [credentials](auth-proxy.md#credential-sources).
- Run these examples in one bash session.
- Both listeners bind to loopback.

```bash
read -r -s -p 'Proxy admin secret: ' PROXY_ADMIN_SECRET
printf '\n'
export PROXY_ADMIN_SECRET

oaiso proxy serve --role auth \
  --listen 127.0.0.1:18080 --admin-listen 127.0.0.1:18081 \
  --admin-secret "$PROXY_ADMIN_SECRET" &
PROXY_PID=$!

export PROXY_DATA_URL=http://127.0.0.1:18080
export PROXY_ADMIN_URL=http://127.0.0.1:18081
```

- Use `serve` for this local example: `proxy run` does not publish the data port to the host.
- Do not expose plain HTTP admin APIs or bearer tokens to other machines.

## 2. Issue a token

```bash
export SESSION_NAME=my-session
TOKEN=$(curl --retry 5 --retry-connrefused --retry-delay 1 -fsS \
  -X POST "$PROXY_ADMIN_URL/admin/issue" \
  -H 'Content-Type: application/json' \
  -H "X-Proxy-Admin-Secret: $PROXY_ADMIN_SECRET" \
  -d "{\"container\":\"$SESSION_NAME\",\"ttl\":3600}" \
  | python3 -c 'import json,sys; print(json.load(sys.stdin)["token"])')
export TOKEN
```

Do not print tokens to logs. `/health` checks the proxy process, not this token or upstream AI connectivity.

## 3. Use a separate client directory

- Create a separate Codex home so the original login file is not overwritten.
- Parse the token response as JSON rather than inserting it into executable code.

```bash
export CODEX_HOME=$(mktemp -d)
chmod 700 "$CODEX_HOME"

python3 - <<'PY'
import json
import os
from pathlib import Path
from urllib.request import Request, urlopen

base = os.environ['PROXY_DATA_URL']
request = Request(base + '/token', headers={'X-Codex-Token': os.environ['TOKEN']})
with urlopen(request, timeout=10) as response:
    data = json.load(response)
home = Path(os.environ['CODEX_HOME'])
config = 'openai_base_url = ' + json.dumps(base + '/v1') + '\n'
if 'oauth_access_token' in data:
    auth = {
        'auth_mode': 'chatgpt',
        'OPENAI_API_KEY': None,
        'tokens': {
            'access_token': data['oauth_access_token'],
            'refresh_token': '',
            'id_token': data.get('oauth_id_token', ''),  # Fresh dummy JWT, not the real ID token.
            'account_id': data.get('oauth_account_id', ''),
        },
        'last_refresh': data.get('oauth_last_refresh', ''),
    }
    path = home / 'auth.json'
    with path.open('x', opener=lambda p, flags: os.open(p, flags, 0o600)) as out:
        json.dump(auth, out)
    config += 'chatgpt_base_url = ' + json.dumps(base + '/chatgpt/') + '\n'
(home / 'config.toml').write_text(config)
PY

if [ -f "$CODEX_HOME/auth.json" ]; then
  unset OPENAI_API_KEY
  export CODEX_REFRESH_TOKEN_URL_OVERRIDE="$PROXY_DATA_URL/oauth/token?cdx=$TOKEN"
else
  export OPENAI_API_KEY="$TOKEN"
  unset CODEX_REFRESH_TOKEN_URL_OVERRIDE
fi
codex
```

- OAuth uses the new auth.json; API-key mode uses the placeholder environment value.
- The API destination is explicit in config.toml.
- See [verified client versions](../testing.md) (Japanese).
- This host process is not isolated from the original home or other host secrets.

## 4. Revoke and stop

```bash
curl -fsS -X POST \
  "$PROXY_ADMIN_URL/admin/revoke?container=$SESSION_NAME" \
  -H "X-Proxy-Admin-Secret: $PROXY_ADMIN_SECRET"
kill "$PROXY_PID"
```

- Revocation and expiry also end active HTTP/WS streams.
- Remove the temporary `CODEX_HOME` after checking its path.
- See the [API reference](auth-proxy/endpoints.md).
