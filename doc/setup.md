# 導入と更新の自動化

自動配置の対象は **Ubuntu 26.04 / Linux x86_64 / systemd / KVM / cgroup v2** です。
その他のLinuxは[手動配置](kata.md)を使ってください。Windows・macOSへの実行ノード配置は対象外です。

## 初めて配置する

ソースのルートで、Go・Make・Python 3.11以上を使って同じ版のCLIとhelperをまとめます。
このビルドにはDockerを使いません。

```bash
make node-bundle
./dist/node-bundle/codex-dock setup
```

最初は計画だけを表示します。対象アカウントは既存の通常利用者を指定します。
バンドル内のCLIから実行すると、隣接する `bundle.json` を自動で探します。通常利用者で計画を表示する場合は、その利用者をoperatorに選びます。
計画にはバンドルのパスと版、operator、キャッシュ、オフライン指定が表示されます。
別のCLIや利用者を使う場合は `--bundle PATH --operator USER` を明示してください。
計画を確認したら、同じバンドルのCLIで適用します。

```bash
sudo ./dist/node-bundle/codex-dock setup \
  --operator "$USER" --apply
```

`--apply` では付与先の `--operator` を必ず明示します。適用前に対象アカウントを確認してください。

通常の認証や `run` にはsudoを使いません。
再ログインしてグループを反映した後、ホストのAI認証を用意して実行します。

```bash
codex-dock init
# Claude Codeを使う場合
codex-dock init --agent claude
```

`init` のプロキシイメージを新規ビルドする場合は、このリポジトリのルートで実行します。
詳細は[クイックスタート](getting-started.md)を参照してください。

## 自動で準備するもの

- 不足しているOSパッケージ。既存Dockerや既定containerdのパッケージは置き換えません。
- Kata 4.2.0、containerd 2.4.2、BuildKit 0.34.0、runc 1.4.3の固定配布物。
- guestのPID hook、共有cache無効のDragonball設定、固定資産manifest。
- 製品専用containerd・node・BuildKitサービスと、`codex-dock` グループ。
- IPv4 forwardingと、次回起動用のvsock/netモジュール設定。

外部配布物は[固定URLとSHA256](https://github.com/pacificbelt30/sandbox-codex/blob/main/deploy/kata/assets.lock.json)で確認します。
自作バンドルの `bundle.json` はCLI/helperの組合せと破損を検出するもので、配布元の署名ではありません。
信頼したソースからビルドしたバンドルを使ってください。
管理グループへの所属は、ホストmountと任意イメージを選べるroot相当の権限です。

既存のfirewallルールは変更・flushしません。ホストのforward方針がegressを許可するかは管理者が確認します。
KVMと `cpu memory pids` のcgroup v2 controllerが必要です。ホスト自体がVMの場合はnested virtualizationも必要です。

## 更新する

この自動配置で管理している環境では、同じ `setup --apply` を新しいバンドルで実行します。

1. 作業中の成果を保存する。
2. 旧版CLIでワーカーを停止・削除し、プロキシも停止・削除する。
3. 新しいソースで `make node-bundle` を実行する。
4. 新しいバンドルの `setup` で計画を確認し、`--apply` する。
5. 再ログインし、通常利用者で `codex-dock init` と実Kata受入試験を実行する。

更新時は、受付サービスとBuildKitを止めた後、停止済みを含む全コンテナ・task・VM予約・VM cgroupが空であることを確認します。
接続不能なcontainerdを「空」として扱いません。作業中のVMやworktreeを自動削除することはありません。
残存資源があれば旧サービスを再開し、回収方法を表示して終了します。

CLI/nodeの配布版が違う場合は検証で拒否します。開発用 `dev` ビルドもAPI・資産・共有設定の確認は行います。
手動配置が既にある場合は上書きを拒否します。[手動更新](kata.md#固定資産の更新)で移行してください。

## 更新に失敗した場合

資産は `/opt/codex-dock/releases/` の新しいディレクトリへ配置します。
既存の固定資産は書き換えません。専用サービスを切り替えて診断に成功してから、導入済み状態を確定します。
切り替え後に失敗した場合は、以前のCLI・設定・サービス定義・forwarding設定・サービスの有効状態を復元します。
OSパッケージの追加、グループの作成・所属追加、ダウンロードキャッシュは残ります。

前の成功した配置へ戻す操作もできます。先にワーカーとプロキシを削除してください。

```bash
codex-dock setup --rollback
sudo codex-dock setup --rollback --apply
```

導入状態と復旧記録は `/var/lib/codex-dock-setup` に保存します。
イメージ、BuildKitキャッシュ、プロジェクトは削除しません。
古いreleaseや失敗したreleaseは調査・復旧用に残します。参照中の資産を手作業で削除しないでください。

## オフライン配置

固定配布物を `assets.lock.json` の `file` 名でキャッシュディレクトリに配置し、`--cache PATH --offline` を指定します。
OSパッケージも事前に用意してください。ネットワークがなくてもchecksum検証は省略しません。
既定キャッシュは `/var/cache/codex-dock-setup` です。ホスト管理者だけが変更できる場所を使います。

proxy/CAが必要な環境では、ダウンロード用curlとaptにホストの設定を用意してください。
TLS検証を無効にするオプションは設けていません。
