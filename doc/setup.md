# 導入と更新の自動化

| 対象 | 方法 |
|---|---|
| Ubuntu 26.04・Linux x86_64・systemd・KVM・cgroup v2 | このページの自動配置 |
| その他のLinux | [手動配置](kata.md) |
| Windows・macOS | 実行ノードの配置対象外 |
| ホスト自体がVM | nested virtualizationも必要 |

## 初めて配置する

### 1. 必要なツールを用意する

- 実行場所：ソースのルート。
- 必要なツール：Go 1.24.7以上・Make・Python 3.11以上。
- ビルド対象：同じ版のCLIとhelperをまとめたバンドル。
- Docker：このビルドには使わない。

### 2. 計画を確認して適用する

```bash
./scripts/install.sh
./scripts/install.sh --apply
```

| 指定 | 動作・条件 |
|---|---|
| 指定なし | バンドルを作成・検証して計画を表示 |
| `--apply` | 計画の成功後、同じバンドルのCLIを `sudo` で実行 |
| `--operator USER` | 権限を与える既存の通常利用者を指定 |
| operator省略 | スクリプトを実行した通常利用者を選択 |
| rootでスクリプトを実行 | `--operator USER` が必須 |
| `--bundle PATH` | 新規ビルドせず、既存の信頼済みバンドルを使用 |
| `--cache PATH` / `--offline` | 計画と適用の両方へ渡す |

### 個別に計画・適用する場合

```bash
make node-bundle
./dist/node-bundle/oaiso setup
```

**計画に表示する項目**

- バンドルのパスと版。
- operator。
- キャッシュのパス。
- オフライン指定。

**バンドルと利用者の選択**

- バンドル内CLI：隣接する `bundle.json` を自動探索。
- 通常利用者による計画：その利用者をoperatorに選択。
- 別のCLI／利用者：`--bundle PATH --operator USER` を明示。
- 適用：同じバンドルのCLIを使用し、付与先の `--operator` を明示。

```bash
sudo ./dist/node-bundle/oaiso setup \
  --operator "$USER" --apply
```

### 3. 通常利用者として認証とイメージを準備する

1. 再ログインしてグループ所属を反映。
2. 利用する認証を準備。
3. 通常利用者で `init` を実行。

```bash
oaiso init
# Claude Codeを使う場合
oaiso init --agent claude
```

- 通常の認証・`run`：`sudo` 不要。
- Proxyイメージの新規ビルド：リポジトリのルートで `init` を実行。
- 詳細：[クイックスタート](getting-started.md)。

## 自動で準備するもの

| 対象 | 内容 |
|---|---|
| OSパッケージ | 不足分を追加。既存Docker／既定containerdのパッケージは置き換えない |
| 固定配布物 | Kata 4.2.0・containerd 2.4.2・BuildKit 0.34.0・runc 1.4.3 |
| guest・固定資産 | PID hook・共有cache無効のDragonball設定・manifest |
| サービス・権限 | 専用containerd・node・BuildKit・`codex-dock` グループ |
| ホスト設定 | IPv4 forwarding・次回起動用vsock/netモジュール |

### 配布物と権限の確認

| 項目 | 確認事項 |
|---|---|
| 外部配布物 | [固定URLとSHA256](https://github.com/pacificbelt30/sandbox-codex/blob/main/deploy/kata/assets.lock.json)で検証 |
| 自作バンドルの `bundle.json` | CLI/helperの組合せ・破損を検出。配布元の署名ではない |
| バンドルの入手元 | 信頼したソースからビルドしたものを使用 |
| 管理グループ | ホストmount・任意イメージを選択できるroot相当の権限 |
| 既存firewall | 変更・flushしない。egressを許可するforward方針は管理者が確認 |
| KVM・cgroup | KVMと、cgroup v2の `cpu memory pids` controllerが必要 |

## 更新する

**対象：この自動配置で管理している環境。**

1. 作業中の成果を保存。
2. 旧版CLIでワーカーとProxyを停止・削除。
3. 新しいソースで `./scripts/install.sh` を実行し、計画を確認。
4. `./scripts/install.sh --apply` で新しいバンドルを適用。
5. 再ログイン後、通常利用者で `oaiso init` と実Kata受入試験を実行。

| 更新時の条件 | 動作 |
|---|---|
| 受付サービス・BuildKit | 先に停止 |
| 空であることの確認対象 | 停止済みを含む全コンテナ・task・VM予約・VM cgroup |
| containerdへ接続不能 | 「空」として扱わない |
| 残存資源あり | 旧サービスを再開し、回収方法を表示して終了 |
| 作業中のVM・worktree | 自動削除しない |
| CLI/nodeの配布版不一致 | 検証で拒否 |
| 開発用 `dev` ビルド | API・資産・共有設定の確認は実施 |
| 既存の手動配置 | 上書きを拒否。[手動更新](kata.md#固定資産の更新)で移行 |

## 更新に失敗した場合

| 対象 | 扱い |
|---|---|
| 新しい資産 | `/opt/codex-dock/releases/` の新規ディレクトリへ配置 |
| 既存の固定資産 | 書き換えない |
| 導入完了の確定 | サービス切り替えと診断の成功後 |
| 切り替え後の失敗 | 以前のCLI・設定・サービス定義・forwarding設定・サービス有効状態を復元 |
| 残る変更 | OSパッケージ追加・グループ作成／所属追加・ダウンロードキャッシュ |

### 前の成功した配置へ戻す

1. ワーカーとProxyを削除。
2. rollback計画を確認。
3. `--apply` で適用。

```bash
oaiso setup --rollback
sudo oaiso setup --rollback --apply
```

| 保存対象 | 場所・扱い |
|---|---|
| 導入状態・復旧記録 | `/var/lib/codex-dock-setup` |
| イメージ・BuildKitキャッシュ・プロジェクト | 削除しない |
| 古い／失敗したrelease | 調査・復旧用に保持 |
| 参照中の資産 | 手作業で削除しない |

## オフライン配置

1. 固定配布物を `assets.lock.json` の `file` 名でキャッシュへ配置。
2. 必要なOSパッケージを事前に準備。
3. `--cache PATH --offline` を指定。

| 項目 | 条件 |
|---|---|
| 既定キャッシュ | `/var/cache/codex-dock-setup` |
| 所有・更新権限 | ホスト管理者だけが変更できる場所を使用 |
| checksum検証 | オフラインでも省略しない |
| proxy・CA | ダウンロード用curlとaptにホストの設定を用意 |
| TLS検証 | 無効化オプションは提供しない |
