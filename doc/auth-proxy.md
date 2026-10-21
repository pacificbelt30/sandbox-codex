# 認証プロキシ

**日本語** | [English](en/auth-proxy.md)

認証プロキシは、ワーカーの短命トークンを検証し、AI APIへ送るときだけ本物の認証情報に差し替えます。
OpenAIとAnthropicの認証を別々に読み込み、同時に提供できます。

## 2つのプロキシの違い

| 名前 | 役割 | 本物の認証情報 |
|---|---|---|
| `codex-auth-proxy` | AI API、トークン取得、管理API | 保持する |
| `codex-http-proxy` | Git・npm・pipなどのHTTP/CONNECT通信 | 保持しない |

一般通信の許可・拒否は[通信ポリシー](proxy-policy.md)で設定します。
AI APIの固定経路には、そのルールを適用しません。

## 起動する

[Kata実行ホスト](kata.md)を用意し、認証を設定してから実行します。
ビルドはGoソースがあるリポジトリのルートで行ってください。

```bash
cxd proxy build
cxd proxy run
```

`proxy run` はcontainerd上で2つのruncコンテナを起動します。
ワーカーはKata VMで動作します。

| 接続元 | 既定の接続先 | 用途 |
|---|---|---|
| ホストCLI | `http://127.0.0.1:18081` | トークン発行・失効・状態確認 |
| ワーカー | `http://codex-auth-proxy:18080` | トークン取得・AI API |
| ワーカー | `http://codex-http-proxy:18082` | 一般通信 |

ワーカーは専用ネットワークの静的hosts経由でプロキシへ到達します。
管理APIは別リスナーに分離し、ワーカーには公開しません。

トークンは値から直接検索します。ワーカー数に比例する全件走査を避け、発行・復元・失効と同じロック内で索引を更新します。
期限・プロバイダー・モデル制限と、既存streamの取消条件は従来どおりです。[性能計測](testing.md#2026-10-21の性能計測)を参照してください。

## 認証ソース

各プロバイダーについて、表の上から順に利用します。

| 優先順位 | OpenAI | Anthropic |
|---|---|---|
| 1 | `~/.codex/auth.json` OAuth | `ANTHROPIC_API_KEY` |
| 2 | `OPENAI_API_KEY` | `CLAUDE_CODE_OAUTH_TOKEN` |
| 3 | `~/.config/codex-dock/apikey` | `~/.claude/.credentials.json` OAuth |
| 4 | `~/.codex/auth.json` APIキー | `~/.config/codex-dock/anthropic-apikey` |

明示したAnthropic APIキーは、保存済みサブスクより優先してAPI課金を選びます。サブスク利用時はunsetしてください。[Claude Code](claude-code.md)にsetup-token、更新、固定CLIの起動手順をまとめています。ケース別プロキシは外部CredentialStoreだけを使い、このホスト側の優先順位へフォールバックしません。OpenAIの保存キーの設定は[authコマンド](commands/auth.md)を参照してください。

認証プロキシは呼出元のUID/GIDで動き、専用HOME `/var/lib/codex-dock-proxy` に認証ファイルを読み取り専用でマウントします。
元ファイルが所有者だけに読み取りを許した `0600` でも利用できます。
元ファイルの権限は変更せず、capabilityも追加しません。
別の利用者でしか読めないファイルやディレクトリは、起動前に拒否します。

## リクエストの流れ

1. ホストCLIがワーカー用の短命トークンを発行する。
2. 起動スクリプトが `/token` からプレースホルダーとクライアント設定用の情報を取得する。
3. エージェントがそのプレースホルダーを付けてAI APIを呼ぶ。
4. プロキシが有効な発行済みトークンかを検証する。
5. 認証ヘッダーを本物に差し替え、固定の上流へ転送する。

実APIキーとOAuthのaccess token・refresh tokenはワーカーに渡しません。
ChatGPT認証では、Codexのアカウント情報読み取りに必要なID tokenを返すことがあります。
ワーカー用auth.jsonのaccess tokenはプレースホルダー、refresh tokenは空文字です。

## 認証と接続の有効期限

| 経路 | ワーカーが送る認証 |
|---|---|
| `/v1/*`、`/chatgpt/*` | `Authorization: Bearer <短命トークン>` |
| `/anthropic/*` | `x-api-key: <短命トークン>` または同じBearer認証 |

HTTPとWebSocketの両方で検証します。
欠落・未発行・期限切れ・失効済み・重複・矛盾する認証は、上流に接続する前に401で拒否します。
失効・期限切れ・トークン置換時は、接続中のHTTP streamとWebSocketも終了します。

TTLの既定値は3600秒です。停止時に失効し、正規の再起動時に復元します。
詳しくは[短命トークン](auth-proxy/tokens.md)を参照してください。

## APIキーとOAuthの転送先

| 認証 | 上流 | 注入する認証 |
|---|---|---|
| OpenAI APIキー | `api.openai.com/v1/` | Bearer APIキー |
| ChatGPT OAuth | `chatgpt.com/backend-api/codex/` | Bearer access token、正しいaccount ID |
| Anthropic APIキー | `api.anthropic.com/` | `x-api-key` |
| Anthropic OAuth | `api.anthropic.com/` | Bearer access token、OAuth用betaヘッダー |

OpenAIの更新要求は `/oauth/token` で中継します。
Anthropicはプロキシが必要に応じて更新します。
いずれも本物のrefresh tokenはプロキシ側で扱います。
実OAuthの長期更新については[検証範囲](testing.md)を確認してください。

## 更新と単体利用

CLIを更新したら、プロキシも[停止・削除・再ビルド・再作成](operations.md#cliプロキシを更新する)してください。
古いプロキシが `worker_token_auth: true` を返さない場合、新しいCLIは接続を拒否します。

ホスト上のローカルプロセスとして使う場合は[プロキシの単体利用](proxy-standalone.md)を参照してください。
APIの仕様は[認証API](auth-proxy/endpoints.md)、実装を拡張する場合は[コードの境界](proxy-policy.md#コードの境界)を参照してください。
