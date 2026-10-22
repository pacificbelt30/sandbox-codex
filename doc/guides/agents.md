# エージェント

**日本語** | [English](../en/guides/agents.md)

- 前提：[実行ホスト](../installation.md)・[認証](authentication.md)・ワーカーイメージの準備。
- 作業場所：エージェントに扱わせるプロジェクトのディレクトリ。

## 対応状況

| エージェント | APIキー | サブスク | 詳細 |
|---|---|---|---|
| Codex CLI | OpenAI APIキー | ChatGPT OAuth | [認証の準備](authentication.md)・[起動オプション](../commands/run.md) |
| Claude Code | Anthropic APIキー | Claude OAuth／setup-token | [Claude Code](../claude-code.md) |
| その他 | 未対応 | 未対応 | [今後の対応](../future-work.md) |

## 起動方法を選ぶ

| 目的 | Codex | Claude Code |
|---|---|---|
| 公式CLIの引数をそのまま渡す | `oaiso codex` | `oaiso claude` |
| image・worktree・mountなどを指定する | `oaiso run --agent codex` | `oaiso run --agent claude` |
| `oaiso run` の既定にする | `oaiso default codex` | `oaiso default claude` |

- `oaiso codex` / `oaiso claude` の引数：エージェントCLIへ渡す。
- Oaisoの起動フラグ：`oaiso run --agent ...` へ指定。
- 直接起動時の独立プロファイル：信頼済み[グローバル設定](../commands/auth.md#日常操作で使うプロファイルを固定する)で選択。
- `oaiso run` の組み込み既定値：認証済みシェル。
- プロジェクトだけの既定変更：`oaiso default codex --local` など。

## 認証・通信・承認の境界

| 項目 | 扱い |
|---|---|
| 実資格情報 | ホスト側Auth Proxyで保持。作業VMへ渡さない |
| ワーカーのAI通信 | 短命トークンで固定AI経路を利用 |
| 一般通信 | 既定拒否。必要な宛先を[許可](../proxy-policy.md) |
| モデル名の指定 | CLIのモデル選択。モデル認可は[Proxy側の制限](../sandbox-features.md#モデル認可と段階的な起動)も必要 |
| エージェントの承認 | [承認モード](../commands/run.md#--approval-mode--承認モードの指定)で指定。Kataの隔離と別の設定 |

## 次の操作

- [ファイル共有・worktree](../workspace.md)。
- [ワーカーの一覧・ログ・停止・再起動](../operations.md)。
- [CLI・イメージの更新](../operations.md#cliプロキシを更新する)。
- [Go APIでのエージェント実行](../go-api.md)。
