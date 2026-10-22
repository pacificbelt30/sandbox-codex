# 旧環境からの移行

- 現行構成は **containerd → Kata runtime-rs → Dragonball** です。
- Docker Engine・Docker CLI・QEMUは実行時に使いません。

## Oaisoへの名称変更

- 正式名称は **御AgentIsolation**、英字表記は **Oaiso**、CLIは **`oaiso`** です。
- 「御」は丁寧さ・敬意を添える接頭辞で、略称は「おあいそ」と読みます。

- `make build` / `make install` とノードセットアップは `oaiso` を提供し、旧 `cxd` / `codex-dock` も互換コマンドとして配置します。
- Bash・Zsh・Fishの補完はそれぞれのコマンド名で生成します。
- `go install` の出力名はGo moduleに従って `codex-dock` のままです。
- 更新済みバイナリの `codex-dock update` は、同じGo binディレクトリへ `oaiso` と `cxd` のaliasを作成します。
- 他のインストーラーが配置したコマンドや無関係なファイルは置き換えません。

- 以下は引き継ぎ用の識別子として維持します。
- 名称変更に伴うコピーや再ログインは不要です。

- グローバル設定と独立認証：`~/.config/codex-dock/`、OAuth更新状態：`~/.local/state/codex-dock/oauth/`
- プロジェクト設定：`codex-dock.toml`、`packages.dock`、既存の環境変数名
- サービス・operator group・socket・イメージ・ワーカーlabel・Proxy wire protocol
- リポジトリURL、Go moduleと `pkg/cxd` のimport・公開API

- 導入済みノードは、通常の[更新手順](setup.md)で新しいCLIを配置してください。
- 新しい `oaiso` の配置先や補完ファイルに管理外のファイルがある場合、セットアップは停止し、退避を求めます。
- 名称変更だけを理由に既存ワーカーや保存領域を削除しないでください。

## 引き継げるもの

| 引き継ぐもの | 扱い |
|---|---|
| プロジェクト、Gitリポジトリ | 同じパスを指定して利用できる |
| Dockerfile、テンプレート、`packages.dock` | BuildKitで再ビルドする |
| 設定ファイル、ホスト認証ファイル | 現行設定を確認して再利用する |
| 旧コンテナID、snapshot、ネットワーク | 自動移植しない。新しいワーカーを作成する |
| 旧QEMU/Docker用manifest、VM予約 | 採用しない。現行資産を検証して作成する |

## 移行する

1. 旧版のCLIで、必要なログと成果を保存する。
2. 旧ワーカーと旧プロキシを停止・削除する。

    - 旧版が管理していたworktreeやネットワークも確認する。

3. [Kata実行ホストの準備](kata.md)に従い、専用containerd・Kata・Dragonball・BuildKit・管理サービスを配置する。
4. 通常利用者で `oaiso runtime-check` を実行する。
5. [クイックスタート](getting-started.md)に沿ってプロキシとイメージを再ビルドし、新しいワーカーを作成する。

- 旧イメージを残す必要がある場合は、移行元で `docker save` したarchiveを専用containerdの `codex-dock` namespaceへimportする方法もあります。
- 通常はDockerfileからの再ビルドを使ってください。

## 接続先と互換モード

- CLIは `/run/codex-dock-node.sock` へ接続します。
- 変更する場合は `CODEX_DOCK_KATA_NODE_SOCKET` に絶対パスを指定します。
- `DOCKER_HOST` やDocker contextでは接続先を変更できません。

- `--runtime runc` は同じLinux/containerdノード上の互換モードです。
- 独立カーネルによる隔離は提供しません。
- Kataが利用できない場合も、自動では切り替えません。

認証プロキシだけを更新する場合は、[日常の運用](operations.md#cliプロキシを更新する)を参照してください。
