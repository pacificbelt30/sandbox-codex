# Auth Proxy 技術仕様

Auth Proxy は codex-dock のセキュリティの核となるコンポーネントです。
コンテナに実際の API キーや OAuth クレデンシャルを渡さず、短命トークンを介して安全に認証情報を提供します。

---

## 概要

```
┌──────────────────────────────────────────────────────────────┐
│  ホスト環境                                                    │
│                                                                │
│  ~/.codex/auth.json         ~/.config/codex-dock/apikey       │
│  (OAuth クレデンシャル)      (API キー)                        │
│          │                           │                         │
│          └───────────┬───────────────┘                         │
│                      ▼                                         │
│           ┌─────────────────────┐                              │
│           │    Auth Proxy        │                              │
│           │  127.0.0.1:PORT     │  ← ランダムポート            │
│           │                     │                              │
│           │  /token  (GET)      │  トークン検証 → クレデンシャル返却
│           │  /health (GET)      │  ヘルスチェック              │
│           │  /revoke (POST)     │  トークン失効                │
│           └─────────┬───────────┘                              │
│                     │ 短命トークン (cdx-xxxx)                   │
│                     ▼                                          │
│           ┌─────────────────────┐                              │
│           │  サンドボックスコンテナ│                              │
│           │  env: CODEX_TOKEN   │                              │
│           │  env: CODEX_AUTH_   │                              │
│           │       PROXY_URL     │                              │
│           └─────────────────────┘                              │
└──────────────────────────────────────────────────────────────┘
```

> **現在の制限 (F-NET-04)**: Auth Proxy は `127.0.0.1` (ホストのループバック) でリッスンしており、
> `dock-net`（192.168.200.0/24）上のコンテナからは到達できません。
> SRS では dock-net 内のアドレスでリッスンする設計になっています。

---

## 認証モード

### API キーモード

`OPENAI_API_KEY` 環境変数または `~/.config/codex-dock/apikey` ファイルから API キーを読み込みます。

```
ホスト                                      コンテナ
  │                                           │
  │  Auth Proxy 起動時に API キーをメモリに保持  │
  │                                           │
  │  短命トークン発行                          │
  │  (CODEX_TOKEN=cdx-<hex64>)               │
  │ ─────────────────────────────────────▶   │
  │                                           │ 起動時 (entrypoint.sh)
  │                                           │
  │  GET /token                               │
  │  X-Codex-Token: cdx-<hex64>              │
  │  ◀───────────────────────────────────── │
  │                                           │
  │  200 OK                                   │
  │  {"api_key": "sk-...", ...}              │
  │ ─────────────────────────────────────▶   │
  │                                           │
  │                                           │ export OPENAI_API_KEY=sk-...
  │                                           │ unset CODEX_TOKEN
  │                                           │ exec codex
```

### OAuth モード（ChatGPT サブスクリプション）

`~/.codex/auth.json` に `refresh_token` フィールドが含まれている場合、または `auth_mode: "chatgpt"` が設定されている場合に自動的に OAuth モードで動作します。

```
ホスト                                      コンテナ
  │                                           │
  │  ~/.codex/auth.json より読み込み:          │
  │   access_token, id_token,                │
  │   refresh_token, account_id,             │
  │   last_refresh                           │
  │                                           │
  │  短命トークン発行                          │
  │  (CODEX_TOKEN=cdx-<hex64>)               │
  │ ─────────────────────────────────────▶   │
  │                                           │ 起動時 (entrypoint.sh)
  │                                           │
  │  GET /token                               │
  │  X-Codex-Token: cdx-<hex64>              │
  │  ◀───────────────────────────────────── │
  │                                           │
  │  200 OK                                   │
  │  {"oauth_access_token": "ey...",         │
  │   "oauth_id_token": "ey...",             │
  │   "oauth_refresh_token": "rt-...",       │
  │   "oauth_account_id": "...",             │
  │   "oauth_last_refresh": "..."}           │
  │ ─────────────────────────────────────▶   │
  │                                           │
  │                                           │ /home/codex/.codex/auth.json を生成
  │                                           │ (ホストの auth.json と同等の内容)
  │                                           │ unset CODEX_TOKEN
  │                                           │ exec codex
```

> ⚠️ **セキュリティ警告: OAuth モードでは refresh_token がコンテナに渡されます**
>
> Codex CLI v0.110.0 以降は `auth.json` の全フィールド（`id_token`、`refresh_token`、`account_id` 等）を必須とするため、
> OAuth モードではこれらすべてをコンテナに渡しています。
>
> これは **bind-mount と実質的に同等のセキュリティレベル** です。
> `refresh_token` を持つコンテナは、ホストと同等の権限でトークンをリフレッシュできます。
>
> **リスクの低減策**:
> - コンテナの `--pids-limit` と `--cap-drop ALL` による実行環境の制限は引き続き有効
> - `dock-net` による ICC 無効・ホストアクセス制限も有効
> - 短いセッションで使用し、不要なコンテナは即座に停止・削除する
> - 可能であれば OAuth の代わりに OpenAI API キー (`sk-...`) を使用する（API キーモードでは `refresh_token` は存在しないため安全）

---

## API エンドポイント仕様

### `GET /token` — クレデンシャル取得

コンテナが短命トークンを使って認証情報を取得するエンドポイントです。

**リクエスト**

```
GET /token HTTP/1.1
X-Codex-Token: cdx-<64桁の16進数>
```

| 項目 | 内容 |
|---|---|
| メソッド | `GET` |
| 認証ヘッダー | `X-Codex-Token: <token>` |
| ボディ | なし |

**レスポンス（API キーモード）**

```json
HTTP/1.1 200 OK
Content-Type: application/json

{
  "api_key": "sk-...",
  "container_name": "codex-brave-atlas"
}
```

**レスポンス（OAuth モード）**

```json
HTTP/1.1 200 OK
Content-Type: application/json

{
  "oauth_access_token": "eyJhbGci...",
  "container_name": "codex-calm-beacon"
}
```

**エラーレスポンス**

| ステータス | 条件 |
|---|---|
| `400 Bad Request` | `X-Codex-Token` ヘッダーが欠如 |
| `401 Unauthorized` | トークンが無効・期限切れ |
| `405 Method Not Allowed` | GET 以外のメソッド |

---

### `GET /health` — ヘルスチェック

```
GET /health HTTP/1.1
```

**レスポンス**

```json
HTTP/1.1 200 OK
Content-Type: application/json

{
  "status": "ok",
  "active_tokens": 3
}
```

| フィールド | 説明 |
|---|---|
| `status` | 常に `"ok"` |
| `active_tokens` | 現在有効なトークン数 |

---

### `POST /revoke` — トークン失効

```
POST /revoke?container=<コンテナ名> HTTP/1.1
```

**パラメーター**

| 名前 | 場所 | 説明 |
|---|---|---|
| `container` | クエリパラメーター | 失効対象のコンテナ名 |

**レスポンス**

| ステータス | 条件 |
|---|---|
| `200 OK` | 失効成功 |
| `400 Bad Request` | `container` パラメーターが欠如 |
| `405 Method Not Allowed` | POST 以外のメソッド |

---

## トークンの仕組み

### トークン形式

```
cdx-<64桁の16進数>

例: cdx-a1b2c3d4e5f6789012345678901234567890abcdef1234567890abcdef1234
```

- `crypto/rand` を使って 32 バイトの乱数を生成
- 16 進エンコードして `cdx-` プレフィックスを付与
- 合計 68 文字（`cdx-` + 64 文字の hex）

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

### TTL とトークン期限

| 設定 | デフォルト値 | 変更方法 |
|---|---|---|
| TTL | 3600 秒 (1 時間) | `--token-ttl <秒数>` or `config.toml` の `default_token_ttl` |

- 期限切れトークンは 30 秒ごとのバックグラウンドスキャンで削除される
- `RevokeToken()` を呼ぶと即時削除される

> **既知の問題 (F-AUTH-04)**: コンテナ停止時に `RevokeToken()` が自動的に呼ばれない実装ギャップがあります。
> コンテナが停止した後も、TTL 切れまでトークンがメモリ上に残り続けます。

---

## クレデンシャルの優先順位

Auth Proxy が API キーを探す順番は以下の通りです：

```
1. OPENAI_API_KEY 環境変数
       │
       │ なければ
       ▼
2. ~/.config/codex-dock/apikey
   (codex-dock auth set で保存したキー)
       │
       │ なければ
       ▼
3. ~/.codex/auth.json
   (Codex CLI が生成する認証ファイル)
```

OAuth モードは `~/.codex/auth.json` に `refresh_token` フィールドがある場合に有効になります。

---

## セキュリティ考慮事項

### 実装済みの保護

| 保護 | 実装 | 詳細 |
|---|---|---|
| API キーの隔離 | ✅ | コンテナに直接渡さず Auth Proxy 経由でのみ提供 |
| refresh_token の保護 | ⚠️ **OAuth モードでは無効** | API キーモードでは不要。OAuth モードでは Codex CLI の要件により全フィールドをコンテナに渡す |
| 短命トークン | ✅ | TTL 付き、使用後は無効化可能 |
| クレデンシャルのログ出力禁止 | ✅ | 認証情報を stdout/stderr に出力しない |
| `auth.json` の bind mount 禁止 | ✅ (API キーモード) / ⚠️ (OAuth モード) | OAuth モードでは auth.json と同等の内容をコンテナ内に書き込む |

### 既知の問題・制限

| ID | 問題 | 影響度 | 詳細 |
|---|---|---|---|
| F-AUTH-04 | コンテナ停止時のトークン自動失効なし | 中 | TTL 切れまでトークンが残存する |
| F-NET-04 | コンテナから Auth Proxy に到達不可 | 高 | 127.0.0.1 はコンテナから到達できない |
| NF-SEC-01 | 平文 HTTP 通信 | 高 | TLS または UNIX ソケットが未実装 |
| F-AUTH-06 | コンテナ ID による照合なし | 中 | トークンがコンテナ名と紐付けられているが、コンテナ ID との照合なし |

### セキュリティ改善の方向性 (SRS 記載)

- **NF-SEC-01**: Auth Proxy を UNIX ドメインソケット or TLS に移行
- **F-NET-04**: Auth Proxy を dock-net の特定アドレスでリッスン（例: 192.168.200.1:PORT）
- **F-AUTH-04**: `sandbox.Manager.Stop()` から `proxy.RevokeToken()` を呼ぶ
- **F-AUTH-06**: トークン発行時にコンテナ ID も記録し、リクエスト時に Docker API で照合

---

## 実装コード早見表

```go
// Auth Proxy の作成
proxy, _ := authproxy.NewProxy(authproxy.Config{
    TokenTTL: 3600,
    Verbose:  true,
})

// 起動
proxy.Start()

// トークン発行（コンテナ起動前に呼ぶ）
token, _ := proxy.IssueToken("my-container", 3600)

// エンドポイント確認
fmt.Println(proxy.Endpoint()) // "http://127.0.0.1:XXXXX"

// トークン失効（コンテナ停止時に呼ぶべき）
proxy.RevokeToken("my-container")

// プロキシ停止（全トークンをクリア）
defer proxy.Stop()
```

---

## 環境変数（コンテナ内）

コンテナには以下の環境変数が注入されます：

| 変数名 | 内容 | 使用後の処理 |
|---|---|---|
| `CODEX_AUTH_PROXY_URL` | `http://127.0.0.1:PORT` | `unset`（entrypoint.sh が削除） |
| `CODEX_TOKEN` | `cdx-<hex64>` | `unset`（entrypoint.sh が削除） |

> entrypoint.sh は認証取得後にこれらの環境変数を `unset` します。
> Codex CLI が実行される時点では、これらの変数は存在しません。
