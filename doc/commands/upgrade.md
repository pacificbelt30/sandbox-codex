# `oaiso upgrade` — 配布版の更新

**日本語** | [English](../en/commands/upgrade.md)

- **開発版の機能です。公開済みv0.0.2では利用できません。** v0.0.2は[配布インストーラー](../setup.md#更新する)を再実行してください。
- Goの追加は不要です。既存の配布インストーラーを使い、ホスト全体の更新手順を案内します。

## 更新計画を確認する

```bash
oaiso upgrade
oaiso upgrade --version v0.0.2
```

- どちらかを選びます。版を省略するとGitHubで最新の安定版を調べます。
- 指定版の`install.sh`と`SHA256SUMS`をHTTPSで取得し、ハッシュ一致後に計画を表示します。
- ハッシュは破損・配布物の不一致を検出するものです。独立した発行者署名ではありません。
- この段階ではサービス・製品バイナリの配置を変更しません。必要なuv・Pythonの利用者領域への準備と、配布物のダウンロードは行います。

```text
Installed CLI: dev
Target release: v0.0.2

Host update: CLI, node, helpers and pinned runtime assets.
Images: rebuild the default worker after the host update; prepare proxies with the new CLI.
Custom images/templates: review separately.
Before applying: save work, then stop/remove retained workers and proxies with the installed CLI.
...
Plan only. Apply the same release with: oaiso upgrade --version v0.0.2 --apply
```

- 上は出力例です。実際の版・計画は配置状態で変わります。
- 選択したプロファイル保存rootに暗号化ファイルがあると、非対応のv0.0.2以前への適用を拒否します。別の保存rootは`--auth-profile-dir`で指定してください。

## 適用する

1. 計画と[リリース情報](../release-notes.md)を確認。
2. 作業を保存し、既存CLIでワーカー・Proxyを停止して削除。
3. 計画に表示された版を指定して適用。

```bash
oaiso upgrade --version vX.Y.Z --apply
```

- `vX.Y.Z`は確認した配布版に置き換えます。
- 通常ユーザーで実行します。必要な管理操作はインストーラーが`sudo`で実施します。
- 実行中・保持中の資源や不整合があると、既存インストーラーの検査が適用を止めます。
- 失敗時の復旧：[setup](../setup.md)。

## 更新する範囲と順番

| 順番 | 対象 | 操作 |
|---|---|---|
| 1 | CLI・node・guard・IO logger・固定runtime資産 | `upgrade --apply`の配布インストーラー |
| 2 | デフォルト作業イメージ | 更新後のCLIで`oaiso build --template plain --tag codex-dock:latest` |
| 3 | 認証Proxy | 更新後のCLIで`oaiso init --agent codex`または`--agent claude` |
| 4 | 独自イメージ・テンプレート | 差分を確認して個別にビルド |
| 5 | 動作 | 新しいワーカーで接続・推論を確認 |

- イメージを実行中の古いCLIから自動ビルドしません。手順2以降は更新後のCLIで実施します。
- `oaiso update`は[Goで導入したCLIだけ](update.md)を更新する別のコマンドです。
