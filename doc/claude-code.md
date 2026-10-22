# Claude Code

**日本語** | [English](en/claude-code.md)

Claude CodeをKata VM内で実行できます。Anthropic APIキーによる従量課金と、Claude Codeを利用できるサブスクリプションのOAuth認証に対応します。実資格情報は認証プロキシだけが保持し、公式Claude CLIにはワーカーごとの短命プレースホルダーを渡します。

## APIキーで使う

ホストに[Kata実行環境](setup.md)を準備した後、プロジェクトのディレクトリで実行します。

```bash
export ANTHROPIC_API_KEY='your-api-key'
unset CLAUDE_CODE_OAUTH_TOKEN
oaiso init --agent claude
oaiso claude
# 対話なしの実行
oaiso claude -p 'このプロジェクトの構成を説明して' --model claude-sonnet-4-6
```

`oaiso claude` はClaude CLIの引数をそのまま渡します。`-p`、`--output-format`、`--resume`、`--model`などはClaudeのオプションです。Oaiso側のイメージ・プロファイル・マウント設定にはグローバル設定またはプロジェクトの `codex-dock.toml` を使います。既存の `oaiso run --agent claude` と `oaiso default claude` も使えます。`run --task` は初期プロンプトであり、非対話実行には `oaiso claude -p` を使ってください。

## サブスクリプションで使う

ホストの公式Claude Codeでログインします。

```bash
unset ANTHROPIC_API_KEY CLAUDE_CODE_OAUTH_TOKEN
claude auth login
oaiso init --agent claude
oaiso claude
```

ホストの `~/.claude/.credentials.json` に保存された `claudeAiOauth` をプロキシが読みます。macOS Keychainや独自の `CLAUDE_CONFIG_DIR` からの自動取り込みはありません。ファイルがない環境では、ホストで `claude setup-token` を実行し、その出力をホストの `CLAUDE_CODE_OAUTH_TOKEN` に設定してから `oaiso init --agent claude` を実行できます。実トークンをDockerfile、プロジェクト設定、ワーカーへの追加環境変数に入れないでください。

認証ソースは `ANTHROPIC_API_KEY` → `CLAUDE_CODE_OAUTH_TOKEN` → `~/.claude/.credentials.json` → 保存済みAnthropic APIキーの順です。明示したAPIキーがあるとAPI課金になります。サブスク利用時はそれをunsetしてください。稼働済みプロキシの認証方式は、ホストの環境変数を変更するだけでは切り替わりません。

サブスク利用の可否、対象モデル、上限は契約とAnthropic側の判定に従います。本機能は公式Claude Codeの実行用です。サブスク認証を汎用のAnthropic API資格情報として提供するものではありません。[公式認証ガイド](https://code.claude.com/docs/en/authentication)と[環境変数の仕様](https://code.claude.com/docs/en/env-vars)を参照してください。

## 認証の分離と更新

| 認証 | ワーカー内 | プロキシから上流へ |
|---|---|---|
| APIキー | `ANTHROPIC_API_KEY=cdx-…` | 実 `x-api-key` |
| サブスク | `CLAUDE_CODE_OAUTH_TOKEN=cdx-…`、APIキーは空 | 実Bearer access token、OAuth betaヘッダー |

`ANTHROPIC_BASE_URL` はワーカーネットワーク内の `/anthropic` を指します。クライアントのAPI版・betaヘッダー、Messages本文、ツール結果、SSEを保持します。`/token` の取得はCodex用で、Claudeはこの表の環境変数を使います。一般通信を止める `--no-internet` でも固定AI経路は使えます。

固定版を維持するため `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1` を設定し、自動更新・テレメトリなどの非必須通信を無効にします。この設定でRemote Control等も利用できません。ログインと再認証はホストで行い、VM内の `/login` は使わないでください。

ログインファイルにrefresh tokenと期限があれば、プロキシが期限の60秒前から更新します。更新要求は最大20秒、応答は最大1MiBです。更新・保存に失敗した要求は推論へ進まず502になります。外部CredentialStoreの保存失敗・版競合はケースの新規要求と既存streamを失効させます。更新結果の保存先と再接続時の扱いは[認証プロキシ](auth-proxy.md)と[ケース別認証](credential-store.md)を参照してください。

`setup-token` のaccess tokenだけを使う場合は自動更新しません。期限切れ時はホストで再発行し、プロキシを再作成します。ワーカーの仮トークンTTLも独立に適用され、既定は3600秒です。停止時に失効し、管理された再起動で復元します。

## Go APIから実行する

[Go API](go-api.md)の `Agent: "claude"` と `AgentArgs` でCLIをメインプロセスとして起動できます。待機中ワーカーで実行する場合は、`Create` → `Start` の後に接続ヘルパーを使います。

```go
command, err := client.ClaudeCommand(ctx, worker.ID)
if err != nil {
    return err
}
command.Argv = append(command.Argv, "-p", "構成を説明して", "--model", "claude-sonnet-4-6")
code, err := client.Exec(ctx, worker.ID, cxd.ExecRequest{
    Argv: command.Argv, Env: command.Env, Stdout: output, Stderr: output,
})
```

`ClaudeCommand` は保存済みのAPIキー／OAuth方式を選び、競合する認証・クラウドプロバイダーの環境変数を消します。`--setting-sources ""` によりユーザー／プロジェクト設定に依存しません。`Env` には仮トークンがあるためログに出さないでください。`Connections` は対応する環境変数名とヘッダーを返し、トークン値は返しません。外部Storeの `AnthropicOAuth.RefreshToken` は省略可能です。

モデル制限を使うときは正確な上流モデルIDを指定します。Messagesとcount_tokensの `?beta=true` は対応していますが、その他のクエリ・未知の経路・WebSocketは拒否します。Claudeが補助要求に別モデルを使う場合は、そのモデルも許可する必要があります。`--bare` は公式CLIがOAuthを読まないモードのため、サブスク起動には使えません。

## イメージの更新

組み込みワーカーはClaude Code **2.1.306** を固定しています。新CLIとプロキシを同じソース版で配置し、リポジトリのルートで次を実行します。稼働中ワーカーは先に終了してください。

```bash
oaiso build -t codex-dock:latest -f docker/sandbox/Dockerfile
oaiso proxy build
oaiso proxy stop
oaiso proxy rm
oaiso proxy run
```

既存ワーカーは古いイメージ・認証用環境変数を保持するため、新しく作成してください。独自テンプレートがある場合は、entrypointとClaudeの版も更新します。`CODEX_DOCK_CLAUDE_VERSION=2.1.306` でビルド時の版を明示できます。別の版を選んだ場合は、固定CLIの受入テストを更新して確認してください。

検証済み範囲と実アカウントで未確認の項目は[テスト記録](testing.md#claude-codeの受入)を参照してください。
