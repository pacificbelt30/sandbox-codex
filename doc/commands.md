# コマンドリファレンス

> **日本語** | [English](en/commands.md)

cxd の全コマンドリファレンスです。各コマンドの詳細は以下のページを参照してください。

---

## コマンド一覧

| コマンド | 説明 | 詳細 |
|---|---|---|
| [`cxd codex`](commands/run.md#codex-cli-を直接起動する) | Codex CLIの引数をそのまま渡して起動 | カレントディレクトリを共有・直接exec |
| [`cxd run`](commands/run.md) | サンドボックスコンテナの起動 | オプション・承認モード・並列実行・パッケージ管理 |
| [`cxd default`](configuration.md#プロジェクト設定) | `cxd run` の既定エージェントを表示・変更 | グローバル設定または `--local` |
| `cxd project init/check` | プロジェクト設定の作成と検証 | [設定リファレンス](configuration.md#プロジェクト設定) |
| [`cxd setup`](setup.md) | ホスト導入・更新・復旧 | 計画表示 / --apply / --rollback |
| [`cxd init`](getting-started.md) | 初回起動の準備 | 診断・認証確認・イメージ検査・プロキシ起動 |
| [`cxd runtime-check`](troubleshooting.md#複数の前提条件をまとめて調べる) | 実行環境の診断 | doctor別名・--json・修正手順 |
| [`cxd image check`](commands/build.md#起動前に互換性を調べる) | イメージの事前検査 | platform・必要ツール・entrypoint |
| [`cxd vm`](vm.md) | Kata VMの照合と管理 | ps / inspect / start / stop / rm / prune |
| [`cxd start`](commands/worker.md) | 停止済みワーカーの再起動 | 同じID・認証の復元 |
| [`cxd proxy`](commands/proxy.md) | Auth Proxy の管理 | build / run / serve / stop / rm |
| [`cxd ps`](commands/worker.md#cxd-ps--ワーカー一覧) | ワーカー一覧 | 実行中コンテナの表示 |
| [`cxd stop`](commands/worker.md#cxd-stop--コンテナの停止) | コンテナの停止 | 単体・全停止 |
| [`cxd rm`](commands/worker.md#cxd-rm--コンテナの削除) | コンテナの削除 | 停止済み・強制削除 |
| [`cxd logs`](commands/worker.md#cxd-logs--ログの表示) | ログの表示 | tail・follow |
| [`cxd auth`](commands/auth.md) | 認証管理 | show / set / rotate |
| [`cxd network`](commands/network-cmd.md) | ネットワーク管理 | create / rm / status |
| [`cxd firewall`](commands/firewall.md) | （廃止） | ルータモデルへ移行 |
| [`cxd build`](commands/build.md) | サンドボックスイメージのビルド | Dockerfile の自動検出 |
| [`cxd ui`](commands/ui.md) | TUI ダッシュボード | キーバインド一覧 |
| [`cxd update`](commands/update.md) | cxd 自体の更新 | `go install` によるアップデート |

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
- [Auth Proxy のみを使う](proxy-standalone.md) — `cxd run` を使わずに Codex CLI を設定する

ホスト管理者用の `node serve` と `runtime-lock` は[Kataの配置](kata.md)を参照してください。
