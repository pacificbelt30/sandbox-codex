# プロキシの単体利用

**日本語** | [English](en/proxy-standalone.md)

`codex-dock run` を使わず、ホスト上のクライアントから認証プロキシを使う開発者向けの手順です。
この構成はKata VMやワーカー用のネットワーク隔離を用意しません。
通常利用は[クイックスタート](getting-started.md)を参照してください。

## 1. ローカルで起動する

ホストの[認証ソース](auth-proxy.md#認証ソース)を用意します。
管理secretを入力し、dataとadminをそれぞれloopbackへbindします。以下は同じbashセッションで続けて実行する例です。

```bash
read -r -s -p 'Proxy admin secret: ' PROXY_ADMIN_SECRET
printf '\n'
export PROXY_ADMIN_SECRET

codex-dock proxy serve --role auth \
  --listen 127.0.0.1:18080 --admin-listen 127.0.0.1:18081 \
  --admin-secret "$PROXY_ADMIN_SECRET" &
PROXY_PID=$!

export PROXY_DATA_URL=http://127.0.0.1:18080
export PROXY_ADMIN_URL=http://127.0.0.1:18081
```

`proxy run` のdataポートはホストへ公開されないため、このローカル例では `proxy serve` を使います。
別マシンへ平文の管理APIやBearerトークンを公開しないでください。

## 2. 短命トークンを発行する

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

トークンをログに出す必要はありません。
`/health` はプロセスの応答であり、このトークンの有効性やAI疎通を保証するものではありません。

## 3. クライアント設定を分ける

元のログインファイルを上書きしないよう、Codex用に別の設定ディレクトリを作ります。
以下のPythonは `/token` のJSONを直接解析し、placeholderだけをクライアント認証として保存します。

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
            'id_token': data.get('oauth_id_token', ''),
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

OAuthでは新しいauth.jsonを使い、APIキーモードでは環境変数のplaceholderを使います。
APIの向き先は `config.toml` に明示します。
クライアントの版による互換性は[検証範囲](testing.md)を参照してください。
このホストプロセス自体を、元のホームや他の秘密から隔離する手順ではありません。

## 4. 終了時に失効させる

```bash
curl -fsS -X POST \
  "$PROXY_ADMIN_URL/admin/revoke?container=$SESSION_NAME" \
  -H "X-Proxy-Admin-Secret: $PROXY_ADMIN_SECRET"
kill "$PROXY_PID"
```

期限切れ・失効で、既存のHTTP streamとWebSocketも終了します。
`CODEX_HOME` に作った一時ディレクトリは、場所を確認してから削除してください。

管理APIの要求・応答は[認証API](auth-proxy/endpoints.md)を参照してください。
