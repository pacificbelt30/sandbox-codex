# 旧環境からの移行

現行構成は **containerd → Kata runtime-rs → Dragonball** です。
Docker Engine・Docker CLI・QEMUは実行時に使いません。

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
2. 旧ワーカーと旧プロキシを停止・削除する。旧版が管理していたworktreeやネットワークも確認する。
3. [Kata実行ホストの準備](kata.md)に従い、専用containerd・Kata・Dragonball・BuildKit・管理サービスを配置する。
4. 通常利用者で `cxd runtime-check` を実行する。
5. [クイックスタート](getting-started.md)に沿ってプロキシとイメージを再ビルドし、新しいワーカーを作成する。

旧イメージを残す必要がある場合は、移行元で `docker save` したarchiveを専用containerdの `codex-dock` namespaceへimportする方法もあります。
通常はDockerfileからの再ビルドを使ってください。

## 接続先と互換モード

CLIは `/run/codex-dock-node.sock` へ接続します。
変更する場合は `CODEX_DOCK_KATA_NODE_SOCKET` に絶対パスを指定します。
`DOCKER_HOST` やDocker contextでは接続先を変更できません。

`--runtime runc` は同じLinux/containerdノード上の互換モードです。
独立カーネルによる隔離は提供しません。Kataが利用できない場合も、自動では切り替えません。

認証プロキシだけを更新する場合は、[日常の運用](operations.md#cliプロキシを更新する)を参照してください。
