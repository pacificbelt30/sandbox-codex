# コマンドリファレンス

> **日本語** | [English](en/commands.md)

codex-dock の全コマンドリファレンスです。各コマンドの詳細は以下のページを参照してください。

---

## コマンド一覧

| コマンド | 説明 | 詳細 |
|---|---|---|
| [`codex-dock codex`](commands/run.md#codex-cli-を直接起動する) | Codex CLIの引数をそのまま渡して起動 | カレントディレクトリを共有・直接exec |
| [`codex-dock run`](commands/run.md) | サンドボックスコンテナの起動 | オプション・承認モード・並列実行・パッケージ管理 |
| [`codex-dock setup`](setup.md) | ホスト導入・更新・復旧 | 計画表示 / --apply / --rollback |
| [`codex-dock init`](getting-started.md) | 初回起動の準備 | 診断・認証確認・イメージ検査・プロキシ起動 |
| [`codex-dock runtime-check`](troubleshooting.md#複数の前提条件をまとめて調べる) | 実行環境の診断 | doctor別名・--json・修正手順 |
| [`codex-dock image check`](commands/build.md#起動前に互換性を調べる) | イメージの事前検査 | platform・必要ツール・entrypoint |
| [`codex-dock vm`](vm.md) | Kata VMの照合と管理 | ps / inspect / start / stop / rm / prune |
| [`codex-dock start`](commands/worker.md) | 停止済みワーカーの再起動 | 同じID・認証の復元 |
| [`codex-dock proxy`](commands/proxy.md) | Auth Proxy の管理 | build / run / serve / stop / rm |
| [`codex-dock ps`](commands/worker.md#codex-dock-ps--ワーカー一覧) | ワーカー一覧 | 実行中コンテナの表示 |
| [`codex-dock stop`](commands/worker.md#codex-dock-stop--コンテナの停止) | コンテナの停止 | 単体・全停止 |
| [`codex-dock rm`](commands/worker.md#codex-dock-rm--コンテナの削除) | コンテナの削除 | 停止済み・強制削除 |
| [`codex-dock logs`](commands/worker.md#codex-dock-logs--ログの表示) | ログの表示 | tail・follow |
| [`codex-dock auth`](commands/auth.md) | 認証管理 | show / set / rotate |
| [`codex-dock network`](commands/network-cmd.md) | ネットワーク管理 | create / rm / status |
| [`codex-dock firewall`](commands/firewall.md) | （廃止） | ルータモデルへ移行 |
| [`codex-dock build`](commands/build.md) | サンドボックスイメージのビルド | Dockerfile の自動検出 |
| [`codex-dock ui`](commands/ui.md) | TUI ダッシュボード | キーバインド一覧 |
| [`codex-dock update`](commands/update.md) | codex-dock 自体の更新 | `go install` によるアップデート |

---

## グローバルオプション

すべてのコマンドで使用できるオプションです。

| オプション | 省略形 | デフォルト | 説明 |
|---|---|---|---|
| `--verbose` | `-v` | `false` | 詳細なログを出力する |
| `--debug` | | `false` | デバッグログを出力する |
| `--config` | | `~/.config/codex-dock/config.toml` | 設定ファイルのパス |

---

## 関連ドキュメント

- [クイックスタート](getting-started.md) — よく使うコマンドパターン
- [設定リファレンス](configuration.md) — コマンドフラグのデフォルト値を変更する
- [Auth Proxy のみを使う](proxy-standalone.md) — `codex-dock run` を使わずに Codex CLI を設定する

ホスト管理者用の `node serve` と `runtime-lock` は[Kataの配置](kata.md)を参照してください。
