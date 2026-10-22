# 上流仕様の静的な変更検知

Codex・Claude Codeの更新候補と、公開API・SDK・認証関連ソースの変更を、ルールベースで検知します。開発用の独立したPythonスクリプトです。ワーカー・Proxyの実行処理や認証設定は変更しません。

この検査は、監視対象の公開定義とCLIの宣言を比較します。実APIへの推論・ログイン・OAuth Refresh、ツール実行、実Kataでの互換性確認は実施しません。これらは[テストと動作確認](testing.md)を参照してください。

## 監視する契約

監視元と使用箇所は `compat/upstream/manifest.json`、基準データは `compat/upstream/baseline.json` にあります。

| 対象 | 取得・比較するもの |
|---|---|
| OpenAI REST API | 公式OpenAPIのPOST `/responses`・`/responses/compact`・`/chat/completions`・`/completions`・`/embeddings`・`/moderations`。要求・応答・参照型・認証方式・接続先 |
| Codex設定 | `model_provider`・`model_providers`・`chatgpt_base_url`、使用中のapps・圧縮・WebSocket設定 |
| Codex CLI | root／execのヘルプ。使用するフラグの存在・引数・説明 |
| Codex app-server | CLIから生成するInitialize／ThreadStart／TurnStart／TurnInterruptのJSON Schema |
| OpenAI SDK | Responsesの要求型とイベント型 |
| Codex認証 | 上流のOAuth保存・更新関連ソースのフィンガープリント |
| Claude Code CLI | root／auth statusのヘルプ。使用するフラグの存在・引数・説明 |
| Anthropic SDK | Messages・count_tokensの要求型とストリームイベント型 |
| Claude Agent SDK | 公開Python SDKの型・フィールド・型エイリアス |
| Claude認証 | 公式CHANGELOGのOAuth・資格情報・ログイン等に言及する行のフィンガープリント |

GitHubの追跡ブランチは取得時にcommit SHAへ解決し、同じリポジトリ・refでは同一commitを使います。スナップショットには取得時刻・固定URL・元成果物のSHA256・CLI版を記録します。CLI版と上流ブランチの観測時点は別です。初期基準のCLIはworker imageに合わせたCodex 0.163.0／Claude Code 2.1.306です。基準の登録は、上流ソース全体の正しさや実サービスとの互換性を保証しません。

## 判定ルール

| レベル | 例 | 更新候補の扱い |
|---|---|---|
| `breaking` | 使用フラグ・操作・プロパティの削除、型変更、要求の必須項目追加、応答の必須項目削除 | 保留 |
| `review` | 認証方式・接続先の変更、SDKの型変更、合成スキーマ・未知の制約変更、認証ソース／リリースノートの変更 | 保留 |
| `info` | 任意プロパティ追加、未使用フラグの変更、CLI版変更、要求のenum拡張 | 情報として記録 |
| `error` | 取得失敗、未解決の参照、解析不能、監視元の欠落、manifest不一致 | 検査失敗 |

要求と応答ではenum・必須項目の互換性方向を分けます。ローカルの`$ref`先を追跡し、循環参照を処理します。外部参照は未対応として失敗させます。仕様の説明文・例・並び順だけの変更を可能な範囲で除外します。型互換性を証明できない変更は保守的に保留します。

Codex app-server・OpenAI SDK・Claude Agent SDKは、本製品がその公開型を直接解釈しているわけではないため`advisory`です。破壊的な差分も`review`として記録し、実行系が壊れたと断定しません。監視範囲はmanifestで選択した操作・ファイル・フラグに限られます。SDKのimport先を全再帰して比較するものではありません。

## ローカルで実行する

オフラインのルール試験・基準整合性確認にはPython 3.12以上を使います。

```bash
make contracts-check
```

上流取得には、YAML読込用の開発依存を追加します。CLIはグローバル環境を更新せず、専用ディレクトリへ配置します。候補版を調べるときはnpmの版指定を変更してください。

```bash
python3 -m pip install -r requirements-contracts.txt
npm install --prefix .kata-verification/contracts-tools --ignore-scripts --no-audit --no-fund \
  @openai/codex@0.163.0 @anthropic-ai/claude-code@2.1.306
node .kata-verification/contracts-tools/node_modules/@anthropic-ai/claude-code/install.cjs
python3 scripts/upstream-contracts.py capture \
  --codex .kata-verification/contracts-tools/node_modules/.bin/codex \
  --claude .kata-verification/contracts-tools/node_modules/.bin/claude \
  --out .kata-verification/contracts/candidate.json
python3 scripts/upstream-contracts.py compare \
  --candidate .kata-verification/contracts/candidate.json \
  --json .kata-verification/contracts/report.json \
  --markdown .kata-verification/contracts/report.md
```

Claudeの`install.cjs`はnative版の実行ファイル選択に使います。このファイルを持たない旧版ではその行を省略します。CLIの調査は新規HOME・設定ディレクトリと環境変数の許可リストで実行します。ホストの実APIキー・OAuth環境変数・認証ファイル・SSH agentを渡しません。公式CLIのhelp・version・スキーマ生成コード自体は実行するため、信頼する配布物を専用環境で調べてください。

`compare`の終了コードは、情報のみなら`0`、確認・修正が必要なら`1`、検査不能なら`2`です。JSON／Markdownの`status`はそれぞれ`pass`／`hold`／`error`です。取得が一部失敗した場合も、失敗元を含む候補JSONを保存します。

## CIと基準の更新

通常のCIではネットワークを使わず、ルールの単体試験と、基準のmanifest・worker DockerfileのCLI固定版との整合性を確認します。固定CLI版だけを変更して基準を更新し忘れた場合は失敗します。

`Upstream static contracts`ワークフローは毎日および手動実行で、最新CLIと追跡元を取得します。手動実行ではCLI候補版も指定できます。差分はActionsのサマリーと30日保持のartifactに保存します。`review`・`breaking`・`error`ではジョブを失敗させます。ワークフローの権限は読み取りのみで、基準・image・運用設定の自動更新やPR作成は行いません。

差分の該当箇所・固定URL・型・フラグを確認し、必要な修正と[互換性試験](testing.md)を終えてから、確認した候補JSONを`compat/upstream/baseline.json`へ保存してコミットします。CLIを採用する場合はDockerfileの固定版も合わせます。manifestを変更した場合は、監視範囲変更をレビューしてから新しいmanifestで再取得します。別manifestの基準を互換なものとして扱いません。

## 静的検知で分からないこと

サーバーだけで変更されるトークン失効・Refreshの競合・権限・エラー挙動・ストリーム切断は、この検査だけでは分かりません。Claudeのサブスク認証には、ここで比較できる公開Refreshスキーマがありません。リリースノートに書かれない変更や、監視対象外ファイルの変更も検知できません。認証ソースはコメントだけでも確認対象になる場合があります。

`pass`は監視対象の静的な定義に保留理由がないという意味です。実API／サブスクの接続成功や、安全性全体の保証とは区別してください。

公式仕様: [OpenAI OpenAPIの案内](https://developers.openai.com/api/docs/libraries)、[Codexスキーマ生成](https://learn.chatgpt.com/docs/app-server)、[Claude CLI](https://code.claude.com/docs/en/cli-reference)、[Anthropic SDK](https://platform.claude.com/docs/en/cli-sdks-libraries/overview)。
