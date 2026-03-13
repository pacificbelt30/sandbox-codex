# Auth Proxy のみを使う — Codex 設定ガイド

> **日本語** | [English](en/proxy-standalone.md)

codex-dock の Auth Proxy コンポーネントだけを利用して、Codex CLI を直接（Docker コンテナ外で）実行する方法を説明します。

---

## このガイドの対象

以下のような場合に使用してください：

- Codex CLI をホスト上で直接実行したい（Docker コンテナ不使用）
- 自前の Docker コンテナや CI 環境で Codex CLI を動かしたい
- API キーや OAuth トークンをコンテナ・プロセスに渡さず保護したい
- `codex-dock run` のコンテナ管理機能は不要だが、Auth Proxy のセキュリティだけ使いたい

**通常の `codex-dock run` を使う場合はこのガイドは不要です。**
→ [クイックスタート](getting-started.md) を参照してください。

---

## 前提条件

| 要件 | 確認方法 |
|---|---|
| codex-dock インストール済み | `codex-dock --version` |
| Codex CLI インストール済み | `codex --version` |
| 認証情報が設定済み | `codex-dock auth show` |

---

## Step 1: Auth Proxy を起動する

### 方法 A: Docker コンテナとして起動（推奨）

```bash
# Auth Proxy イメージをビルド（初回のみ）
codex-dock proxy build

# Auth Proxy を起動（--admin-secret を必ず設定すること）
codex-dock proxy run --admin-secret YOUR_SECRET --port 18080
```

起動後、ホスト上から `http://localhost:18080` で管理 API にアクセスできます。

> **セキュリティ**: `--admin-secret` を設定しないと管理 API への認証がなくなります。
> トークンを発行できる相手を限定するために必ず設定してください。

### 方法 B: ローカルプロセスとして起動

```bash
codex-dock proxy serve --listen 127.0.0.1:18080 --admin-secret YOUR_SECRET
```

Codex CLI と同じホストで動かす場合はこちらでも構いません。

---

## Step 2: 短命トークンを発行する

Auth Proxy の管理 API でトークンを発行します。

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

| パラメーター | 説明 |
|---|---|
| `container` | セッション識別名（任意の文字列） |
| `ttl` | トークン有効期間（秒）。省略時はプロキシの `default_token_ttl`（デフォルト 3600 秒） |

---

## Step 3: Codex CLI の環境変数を設定する

発行したトークンと Auth Proxy の URL を Codex CLI に設定します。

### API キーモードの場合

```bash
PROXY_URL="http://localhost:18080"

export OPENAI_API_KEY="$TOKEN"         # プレースホルダートークン（本物のキーではない）
export OPENAI_BASE_URL="$PROXY_URL/v1" # API リクエストをプロキシ経由に

# Codex CLI を起動
codex
```

**環境変数の意味:**

| 変数 | 値 | 説明 |
|---|---|---|
| `OPENAI_API_KEY` | `cdx-xxxx...` | プレースホルダートークン。本物の API キーはプロキシがインジェクト |
| `OPENAI_BASE_URL` | `http://localhost:18080/v1` | Codex CLI の API リクエスト先をプロキシに向ける |

### OAuth モード（ChatGPT サブスクリプション）の場合

OAuth モードでは追加のファイル設定が必要です。

```bash
PROXY_URL="http://localhost:18080"

# Step 2 で発行したトークンで /token エンドポイントからクレデンシャルを取得
RESPONSE=$(curl -sf "$PROXY_URL/token" -H "X-Codex-Token: $TOKEN")

# Codex CLI 用の auth.json を生成
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

# Codex CLI 用の config.toml を生成（chatgpt_base_url をプロキシに向ける）
mkdir -p ~/.config/codex
echo "chatgpt_base_url = \"$PROXY_URL/chatgpt/\"" > ~/.config/codex/config.toml

# OAuth トークンリフレッシュもプロキシ経由に
export CODEX_REFRESH_TOKEN_URL_OVERRIDE="$PROXY_URL/oauth/token?cdx=$TOKEN"

# Codex CLI を起動
codex
```

**設定ファイルの意味:**

| 設定 | 値 | 説明 |
|---|---|---|
| `~/.codex/auth.json`.`tokens.access_token` | `cdx-xxxx...` | プレースホルダー。本物は渡さない |
| `~/.codex/auth.json`.`tokens.refresh_token` | `""` (空) | リフレッシュはプロキシが行うため空 |
| `~/.codex/auth.json`.`tokens.id_token` | 本物の JWT | Codex CLI が claims 抽出に使用（認証 credential ではない） |
| `~/.config/codex/config.toml`.`chatgpt_base_url` | `http://localhost:18080/chatgpt/` | ChatGPT backend-api リクエストをプロキシ経由に |
| `CODEX_REFRESH_TOKEN_URL_OVERRIDE` | `http://localhost:18080/oauth/token?cdx=<token>` | トークンリフレッシュをプロキシ経由に |

> **セキュリティ**: `auth.json` の `access_token` は本物ではありません。
> Codex CLI が API リクエストを送る際のダミー Bearer トークンとして使用され、
> プロキシが本物の access_token に差し替えます。`refresh_token` もプロキシが保持し、コンテナ（プロセス）には渡しません。

---

## Step 4: 動作確認

Auth Proxy のヘルスエンドポイントでトークンが有効であることを確認します。

```bash
# ヘルスチェック（アクティブトークン数が 1 以上になっていれば OK）
curl -s http://localhost:18080/health
# → {"active_tokens":1,"status":"ok"}

# API リクエストが正常にプロキシされることを確認
curl -s "$PROXY_URL/v1/models" \
  -H "Authorization: Bearer $TOKEN" \
  | python3 -m json.tool | head -20
```

---

## Step 5: セッション終了時にトークンを失効させる

セッションが終了したらトークンを明示的に失効させてください。

```bash
curl -sf -X POST \
  "http://localhost:18080/admin/revoke?container=$SESSION_NAME" \
  -H "X-Proxy-Admin-Secret: $ADMIN_SECRET"
```

TTL を設定している場合は期限切れで自動的に失効しますが、不要になったトークンは早めに失効させることを推奨します。

---

## まとめ：モード別設定一覧

### API キーモード

| 設定 | 値 |
|---|---|
| `OPENAI_API_KEY` | プロキシから発行した `cdx-xxxx...` トークン |
| `OPENAI_BASE_URL` | `http://localhost:18080/v1` |

### OAuth モード

| 設定 | 値 |
|---|---|
| `~/.codex/auth.json` | `access_token`: プレースホルダー、`refresh_token`: 空、`id_token`: 本物 |
| `~/.config/codex/config.toml` | `chatgpt_base_url = "http://localhost:18080/chatgpt/"` |
| `CODEX_REFRESH_TOKEN_URL_OVERRIDE` | `http://localhost:18080/oauth/token?cdx=<token>` |

---

## モード判定の確認

現在 Auth Proxy がどのモードで動作しているか確認できます。

```bash
curl -sf http://localhost:18080/admin/mode \
  -H "X-Proxy-Admin-Secret: $ADMIN_SECRET"
# → {"oauth_mode": false}   # API キーモード
# → {"oauth_mode": true}    # OAuth モード
```

---

## トラブルシューティング

### `401 Unauthorized` が返る

- トークンが期限切れになっている → Step 2 でトークンを再発行してください
- `X-Codex-Token` ヘッダーの値が正しくない → `$TOKEN` 変数の値を確認してください

### `curl: Connection refused`

- Auth Proxy が起動していない → `codex-dock proxy run` または `codex-dock proxy serve` を実行してください
- ポート番号が異なる → `PROXY_URL` の値を確認してください

### API キーモードなのに OAuth 用の設定が必要と言われる

- `codex-dock auth show` で認証モードを確認してください
- `~/.codex/auth.json` に `refresh_token` が含まれている場合は OAuth モードになります

---

## 関連ドキュメント

- [Auth Proxy 技術仕様](auth-proxy.md) — プロキシの仕組みと構成図
- [Auth Proxy エンドポイント仕様](auth-proxy/endpoints.md) — 管理 API の詳細仕様
- [Auth Proxy トークン仕様](auth-proxy/tokens.md) — トークンライフサイクル
- [`codex-dock proxy` コマンド](commands/proxy.md) — プロキシの起動方法
- [クイックスタート](getting-started.md) — `codex-dock run` を使う通常の手順
