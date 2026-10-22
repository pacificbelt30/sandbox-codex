# Installation — インストール

**日本語** | [English](en/installation.md)

| 今の状態 | 手順 |
|---|---|
| 新しいUbuntu 26.04ホスト | このページの1〜4 |
| 実行ホストは準備済み | [認証・初回起動](getting-started.md#2-認証方式を選ぶ) |
| インストール済み環境を更新 | [更新手順](setup.md#更新する) |
| その他のLinux | [手動配置](kata.md) |

## 1. 前提条件

| 項目 | 必要条件 |
|---|---|
| OS・CPU | Ubuntu 26.04・Linux x86_64 |
| ホスト機能 | systemd・KVM（`/dev/kvm`）・cgroup v2の `cpu memory pids` |
| 実行アカウント | `sudo` が使える通常ユーザー |
| ビルドツール | Git・Make・Go 1.24.7以上・Python 3.11以上 |
| メモリ目安 | 既定の2ワーカー構成は8 GiB以上。ビルド用の余裕も確保 |
| 通信 | ソース・固定ランタイム配布物・OSパッケージ・コンテナイメージの取得先に到達可能 |
| WSL／VM | 上記要件に加え、nested virtualizationが必要 |

- Docker Engine・Docker CLI・QEMUのインストールは不要。
- operator groupにはホストのroot相当の管理権限を付与。信頼する利用者だけを指定。

## 2. ツールとソースを取得

**通常ユーザーのシェルで実行。**

```bash
sudo apt-get update
sudo apt-get install -y git make python3 golang-go

go version
python3 --version
test -c /dev/kvm && echo 'KVM device found'
cat /sys/fs/cgroup/cgroup.controllers

git clone https://github.com/pacificbelt30/sandbox-codex.git
cd sandbox-codex
```

- 表示された版：Go 1.24.7以上・Python 3.11以上であることを確認。
- cgroupの表示：`cpu memory pids` が含まれることを確認。
- KVMがない場合：[ホスト要件](kata.md#必要なホスト)を満たしてから続行。
- ソース：初回のProxyイメージ作成にも使用。初期設定完了まで保持。

## 3. 計画を確認してインストール

**クローンした `sandbox-codex` ディレクトリで実行。**

```bash
./scripts/install.sh
./scripts/install.sh --apply
```

**計画の出力例（抜粋）**

```text
Plan: verified Ubuntu 26.04 amd64 node; Kata 4.2.0 / Dragonball / containerd 2.4.2 / BuildKit 0.34.0 / runc 1.4.3
Bundle: /home/user/sandbox-codex/dist/node-bundle (version <version>); operator: user; cache: /var/cache/codex-dock-setup; offline: no
```

- `<version>`・パス・利用者名は環境により変わる値。
- 計画だけの実行：末尾に `No host changes. Add --apply as root.` を含む行が表示される。
- 適用成功：末尾に `Node ready.` で始まる行が表示される。再ログイン後、次の手順で確認。

| コマンド・指定 | 動作 |
|---|---|
| 指定なし | 同じ版のCLI・helperをビルドし、検証・配置計画を表示 |
| `--apply` | 計画成功後に `sudo` で配置 |
| operator省略 | スクリプトを実行した通常ユーザーを登録 |
| `--operator USER` | 別の既存通常ユーザーを登録。rootから実行する場合は必須 |

| 配置対象 | 主な場所・名前 |
|---|---|
| CLI | `/usr/local/bin/oaiso` |
| Kata・containerd・BuildKit・runc | `/opt/codex-dock/releases/` の管理資産 |
| 管理サービス | `codex-dock-node`・`codex-dock-containerd`・`codex-dock-buildkit` |
| operator group | `codex-dock` |
| Bash・Zsh・Fish補完 | `/usr/local/share/` 以下 |

- 既存Docker／既定containerdのパッケージは置き換えない。
- ネットワーク・検証・失敗時の復旧：[自動セットアップの詳細](setup.md)。

## 4. 再ログインして確認

1. ログアウトし、登録した通常ユーザーで再ログイン。
2. 下のコマンドを実行。

```bash
command -v oaiso
oaiso --version
id -nG
oaiso runtime-check
```

**CLI・グループの出力例**

```text
$ command -v oaiso
/usr/local/bin/oaiso
$ oaiso --version
oaiso version <version>
$ id -nG
user sudo codex-dock
```

- `$` は入力を表す記号。コピーして実行する場合は上の `bash` ブロックを使用。
- 版・利用者名・他のグループは環境依存。`codex-dock` への所属を確認。
- `runtime-check`：[正常時の全項目と読み方](getting-started.md#1-実行ホストを確認する)。

| 確認対象 | 成功条件 |
|---|---|
| CLI | `oaiso` のパスと版が表示される |
| グループ | `id -nG` に `codex-dock` が含まれる |
| 実行基盤 | `runtime-check` が成功する |

## 次は認証と初回起動

1. [認証方式を選ぶ](getting-started.md#2-認証方式を選ぶ)。独立プロファイル／従来のホスト連携から選択。
2. ソースのルートで `oaiso init`。Claude Codeの場合は `oaiso init --agent claude`。
3. 作業するプロジェクトで `oaiso codex` または `oaiso run --agent claude`。

- 通常の `init`・`run`：`sudo` 不要。
- 実APIキー・OAuthトークン：Auth Proxyで保持。作業VMには短命トークンだけを渡す。
- 一般通信：既定拒否。Git・パッケージ取得が必要なら[宛先を許可](proxy-policy.md)。

## ほかの導入方法

| 方法 | 範囲・参照先 |
|---|---|
| 手動で実行ホストを配置 | [Kata実行ホストの準備](kata.md) |
| 信頼済みバンドルを使用 | [自動セットアップ](setup.md)。`--bundle PATH` |
| オフライン導入 | [オフライン配置](setup.md#オフライン配置) |
| CLI・補助ツール・補完をビルド・配置 | `make build` / `sudo make install`。Kata・管理サービスの配置は含まない |
| 更新・ロールバック | [更新と復旧](setup.md#更新する) |
