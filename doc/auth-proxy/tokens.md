# Auth Proxy — トークンの仕組みとセキュリティ

> **日本語** | [English](../../en/auth-proxy/tokens.md)

- [概要・デプロイ](../auth-proxy.md)
- [API エンドポイント仕様](endpoints.md)
- **トークンの仕組みとセキュリティ** ← 本ページ

---

## トークン形式

```
cdx-<64桁の16進数>

例: cdx-a1b2c3d4e5f6789012345678901234567890abcdef1234567890abcdef1234
```

- `crypto/rand` を使って 32 バイトの乱数を生成
- 16 進エンコードして `cdx-` プレフィックスを付与
- 合計 68 文字

---

## トークンライフサイクル

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

### 発行タイミング

| タイミング | 発行方法 |
|---|---|
| `codex-dock run` 実行時 | `proxy.IssueToken()` をコンテナ起動前に呼び出す |
| 外部からの発行 | `POST /admin/issue` エンドポイント経由 |

### 失効タイミング

| タイミング | 失効方法 |
|---|---|
| コンテナ停止時 | `sandbox.Manager.Stop()` が `proxy.RevokeToken()` を呼び出す |
| TTL 超過時 | `expireLoop` が 30 秒ごとに期限切れトークンを削除 |
| 明示的な失効 | `POST /revoke` または `POST /admin/revoke` |
| プロキシ停止時 | `proxy.Stop()` がすべてのトークンをクリア |

### TTL 設定

| 設定 | デフォルト値 | 変更方法 |
|---|---|---|
| TTL | 3600 秒 (1 時間) | `--token-ttl <秒数>` または `config.toml` の `default_token_ttl` |

---

## コンテナへの環境変数

`codex-dock run` がコンテナに注入する環境変数：

| 変数名 | 内容 | 使用後の処理 |
|---|---|---|
| `CODEX_AUTH_PROXY_URL` | `http://<proxy>:<PORT>` | `unset`（entrypoint.sh が削除） |
| `CODEX_TOKEN` | `cdx-<hex64>` — `/token` 取得用 | `unset`（entrypoint.sh が削除） |
| `OPENAI_BASE_URL` | `http://<proxy>:<PORT>/v1` | 常時有効（Codex CLI が参照） |
| `CODEX_REFRESH_TOKEN_URL_OVERRIDE` | `http://<proxy>:<PORT>/oauth/token?cdx=<token>` | OAuth モード時のみ設定 |

OAuth モードでは `entrypoint.sh` が以下のファイルを生成します：

| ファイル | 内容 |
|---|---|
| `/home/codex/.codex/auth.json` | `access_token`：プレースホルダー、`id_token`：本物、`refresh_token`：空文字 |
| `/home/codex/.config/codex/config.toml` | `chatgpt_base_url = "http://<proxy>:<PORT>/chatgpt/"` |

---

## クレデンシャルの優先順位（API キーモード）

```
1. OPENAI_API_KEY 環境変数
2. ~/.config/codex-dock/apikey  (codex-dock auth set で保存)
3. ~/.codex/auth.json
```

OAuth モードは `~/.codex/auth.json` に `refresh_token` または `auth_mode: "chatgpt"` がある場合に有効になります。

---

## セキュリティ考慮事項

### 実装済みの保護

| 保護 | 実装 | 詳細 |
|---|---|---|
| API キーの隔離 | ✅ | コンテナには `CODEX_TOKEN` と同じプレースホルダーのみ渡す。本物のキーはプロキシがインジェクト |
| access_token の隔離 | ✅ | OAuth モードでも本物の access_token はコンテナに渡らない。プレースホルダーをプロキシが差し替え |
| refresh_token の保護 | ✅ | コンテナに渡さない。リフレッシュは `/oauth/token` 中継で実現 |
| 短命トークン | ✅ | TTL 付き、コンテナ停止時に即時失効 |
| API トラフィックの中継 | ✅ | `/v1/` と `/chatgpt/` のリバースプロキシで外部 API への直接通信を排除 |
| クレデンシャルのログ出力禁止 | ✅ | 認証情報を stdout/stderr に出力しない |
| `auth.json` の bind mount 禁止 | ✅ | コンテナ内の auth.json は access_token がプレースホルダーの安全なコピー |

### 既知の問題・制限

| ID | 問題 | 影響度 | 詳細 |
|---|---|---|---|
| NF-SEC-01 | 平文 HTTP 通信 | 高 | TLS または UNIX ソケットが未実装。Docker 内部通信のみで使用することを想定 |
| F-AUTH-06 | コンテナ ID による照合なし | 中 | トークンはコンテナ名と紐付けられているが、コンテナ ID との照合なし |

---

## 実装コード早見表

```go
// Auth Proxy の作成
proxy, _ := authproxy.NewProxy(authproxy.Config{
    TokenTTL:    3600,
    Verbose:     true,
    ListenAddr:  "0.0.0.0:0",
    AdminSecret: "my-secret",
})

// 起動
proxy.Start()

// トークン発行（コンテナ起動前に呼ぶ）
token, _ := proxy.IssueToken("my-container", 3600)

// エンドポイント確認
fmt.Println(proxy.Endpoint())           // "http://127.0.0.1:XXXXX"
fmt.Println(proxy.ContainerEndpoint())  // "http://host.docker.internal:XXXXX"

// OAuth モード判定
if proxy.IsOAuthMode() {
    // CODEX_REFRESH_TOKEN_URL_OVERRIDE を設定する
}

// トークン失効（コンテナ停止時に呼ぶ）
proxy.RevokeToken("my-container")

// プロキシ停止（全トークンをクリア）
defer proxy.Stop()
```

---

## 関連ドキュメント

- [Auth Proxy 概要・デプロイ](../auth-proxy.md)
- [API エンドポイント仕様](endpoints.md)
- [セキュリティ設計](../security.md)
- [Auth Proxy のみを使う（Codex 設定ガイド）](../proxy-standalone.md)
