# Future work

Codex CLI・Claude Codeに加え、以下のコーディングエージェントへの対応を今後の作業として管理します。記載したエージェントは未対応の候補です。採用・実装順・提供時期は未確定です。

## その他のコーディングエージェントへの対応

| エージェント | 対応時に確認・実装すること |
|---|---|
| [Gemini CLI](https://github.com/google-gemini/gemini-cli) | CLI起動Adapter、認証方式、Gemini向けのProxy接続、headless実行 |
| [GitHub Copilot CLI](https://github.com/github/copilot-cli) | Copilot固有の認証・接続経路、GitHub連携、対話／非対話実行 |
| [OpenCode](https://opencode.ai/docs/) | CLI起動Adapter、provider設定、Proxy接続、非対話実行 |
| [Aider](https://aider.chat/) | API接続とProxy設定、Git変更・コミットの扱い、非対話実行 |
| [Cursor CLI](https://cursor.com/docs/cli/overview) | CLI起動Adapter、認証・接続先設定、headless実行 |
| [Cline CLI](https://docs.cline.bot/usage/cli-overview) | CLI起動Adapter、provider・認証設定、headless実行、ツール実行 |
| [Kilo Code CLI](https://kilo.ai/docs/code-with-ai/platforms/cli) | CLI起動Adapter、provider・認証設定、Proxy接続 |
| [Goose](https://github.com/aaif-goose/goose) | CLI起動Adapter、provider・認証設定、ツール／MCP連携 |

エディター拡張やクラウド版の対応とは分け、まずローカルのCLIをKataで実行する形を調査します。API方式・サブスク方式の可否はエージェントと提供元ごとに確認し、両方式への対応を一律には約束しません。

## 追加対応の共通条件

- 認証Proxyを維持し、実APIキー・Access token・Refresh token・ID tokenを作業VMへ渡さない。作業VMには短命・実行別・Proxy限定の仮トークンだけを渡す。
- ホストCLIの認証ファイルや継承環境を自動流用せず、本製品の独立した認証profileを使う。認証方式・資格情報の保管・Proxy経由の利用が公式条件と両立しない場合は、対応を保留する。
- 認証専用Kataを使う場合は秘密を永続化せず、終了・取消・障害時に一時状態を破棄する。資格情報を保持する常設VMへの置き換えは行わない。
- 現行の実IDトークン返却の例外解消と、独立した認証管理基盤を優先する。新しいエージェントのために秘密分離やモデル認可を緩めない。
- CLI版・配布物・imageを固定し、ユーザー／プロジェクトの設定に依存しない明示的な起動設定を提供する。[上流仕様の変更検知](upstream-compatibility.md)にも監視対象を追加する。
- 実Kataで起動、ツール実行、ストリーム、取消、停止・再起動、失効、ログ・ファイル・snapshotへの資格情報漏出を確認する。対応済みの機能と未対応の制約を[動作確認](testing.md)に記録する。
- 導入前にCLI配布物・SDK・保存する基準データのライセンスと、認証・再配布・サブスク利用の条件を確認する。
