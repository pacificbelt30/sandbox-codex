# 短命トークン

**日本語** | [English](../en/auth-proxy/tokens.md)

- ワーカーが持つ `cdx-...` は、本物のAPIキーの代わりにプロキシへ提示するBearerトークンです。
- この値を持つ相手は、有効期限内にプロキシのAI経路を使えます。

## 形式と有効期限

- `crypto/rand` で生成した32バイトを16進表記にし、`cdx-` を付けます。
- 形式は `cdx-` + 64桁の16進数、合計68文字です。

```bash
oaiso run --agent codex --token-ttl 7200
```

- TTLの既定は3600秒です。
- 期限を過ぎると新しいリクエストを拒否し、接続中のHTTP stream・WebSocketも終了します。
- 内部の期限切れレコード整理は30秒ごとですが、認証と接続の期限判定はその整理を待ちません。

## ライフサイクル

| 操作 | トークンの扱い |
|---|---|
| 新規ワーカー起動 | 実行を解放する前に発行・有効化する |
| 停止 | 失効させ、使用中の接続を取り消す |
| 再起動 | 保存済みplaceholderを管理APIで復元し、Kata検証後に処理を解放する |
| 同じ名前へ再発行・restore | 古いリースと接続を取り消す |
| 削除 | 失効と資源回収を行う。失敗は再試行記録に残す |
| プロキシ停止 | 全トークンと接続を取り消す。発行台帳はメモリ上にある |

- 停止済みワーカーは `oaiso start NAME` で再起動してください。
- 管理APIのrestoreは、保存済みplaceholderを再有効化するための操作です。
- 実APIキーや、別の稼働ワーカーに割り当て済みのトークンは受け付けません。

## ワーカーに渡す情報

| 情報 | 内容 |
|---|---|
| `CODEX_TOKEN` | `/token` 取得用の短命トークン。起動スクリプトが使用後にunset |
| `CODEX_AUTH_PROXY_URL` | 認証プロキシのURL。起動スクリプトが使用後にunset |
| APIキー用の環境変数 | 実キーではなくplaceholder |
| OAuth用auth.json | access tokenはplaceholder、refresh tokenは空文字 |
| OAuth ID欄 | account ID・plan・FedRAMPフラグだけを持つ新規ダミーJWT。実ID tokenは返さない |
| Codex設定 | `openai_base_url`、OAuth時の `chatgpt_base_url` をプロキシへ向ける |
| `CODEX_REFRESH_TOKEN_URL_OVERRIDE` | OAuth時の更新要求をプロキシへ向ける。短命トークンを含む |

- `HOME` は実行ユーザーに応じて変わります。
- `/home/codex` に固定して扱わないでください。
- 起動後もクライアント用ファイルにはplaceholderが残るため、環境変数をunsetすることだけで秘密を隠す設計ではありません。

## 検証と制限

- 全AI経路のHTTP/WSで、発行済み・有効期限内のトークンを検証します。
- 重複ヘッダーやAnthropicの矛盾したBearer／APIキーヘッダーも拒否します。

- トークンを送信元のコンテナIDへ結び付ける検証はありません。
- ワーカーとプロキシ間の通信は専用ネットワーク上のHTTPです。
- TTLを作業時間に合わせ、不要なトークンは停止・削除で失効させてください。

APIの詳細は[認証API](endpoints.md)、境界は[セキュリティ](../security.md)を参照してください。
