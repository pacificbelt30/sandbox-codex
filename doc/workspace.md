# ファイル共有と所有者

`codex-dock run` は、カレントディレクトリをVM内の `/workspace` に共有します。
ファイルをコピーする方式ではありません。読み書き可能なマウントでは、変更がホストとVMの両方に反映されます。

## 共有するディレクトリを選ぶ

```bash
# カレントディレクトリを共有
codex-dock run

# 別のディレクトリを共有
codex-dock run --project ./my-project

# worktreeを作成して、その別checkoutを共有
codex-dock run --worktree --branch add-tests --new-branch
```

`--project` を指定しても、VM内の作業場所は `/workspace` です。
`--worktree` の場合は作成したworktreeが共有先になります。
その編集は元のcheckoutのファイルへ直接反映されませんが、Gitの共通メタデータとコミットは共有します。

## ホストとVMで変更を共有する

共有にはinline virtio-fsを使い、guest側の `virtio_fs_cache` を `none` に固定します。
Dragonballの別設定も `open,no_writeback_cache` とし、writebackとopen省略を無効にします。
ホストとVMが同じファイルを編集するとき、古い内容がguest cacheへ残ることを避けるためです。
ファイルを開き直す場合だけでなく、開いたままのdescriptorからの読み取りも受入試験で確認します。
キャッシュを使う構成よりファイルアクセスの負荷が増えるため、I/Oの多い処理では性能も確認してください。

旧 `auto` 構成から更新する場合は、[Kataの固定資産更新](kata.md#固定資産の更新)で設定とmanifestを作り直します。

## ホスト側の変更を監視する

ファイルの内容が見えることと、変更通知が届くことは別です。
Kata 4.2.0 / Dragonballの実機試験では、ホストの変更はVM内のinotifyへ届きませんでした。
VM内で行った変更のinotify通知は届きます。
ホスト側のエディターで保存し、VM内の開発サーバーで反映する用途では、監視ツールのポーリングを有効にしてください。

Nodeの `fs.watchFile` でホストの変更を検知できることを確認しています。

```javascript
const fs = require('node:fs');
fs.watchFile('/workspace/example.txt', { interval: 200 }, () => {
  console.log(fs.readFileSync('/workspace/example.txt', 'utf8'));
});
```

フレームワークや監視ライブラリの設定名はそれぞれ異なります。
本製品が全ツールの監視方式を自動変更することはありません。

## 所有者は数値UID/GIDで決まる

既定の `--user current` は、CLIを実行したホスト利用者のUIDと主GIDをVMでも使います。
既存ファイルの所有者を書き換える処理は行いません。

| 指定 | VM内の実行UID/GID |
|---|---|
| 省略、`--user current` | ホストの呼出元UID・主GID |
| `--user dir` | `--project` のディレクトリ所有者のUID/GID |
| `--user 12001:12002` | 明示した数値 |
| `--user codex` | 組み込みイメージの `1001:1001` |

ユーザー名はホストとイメージで違う場合があります。
例えば同じUID 1000を、ホストでは `user`、イメージでは別名として表示することがあります。
比較には数値を使ってください。

```bash
id
stat -c '%u:%g %a %n' example.txt
codex-dock run -- stat -c '%u:%g %a %n' /workspace/example.txt
```

新規ファイルのUIDは作成したプロセス、GIDは通常その主GIDになります。
親ディレクトリにsetgidが付いている場合は、親のGIDを継承します。
パーミッションはVM側プロセスのumaskにも従います。
所有者が同じでも、明示したread-only指定や通常のファイル権限で書き込みを拒否することがあります。

## 読み取り専用と追加マウント

```bash
# workspaceだけを読み取り専用にする
codex-dock run --read-only

# データは読み取り専用、cacheは読み書き可能
codex-dock run --mount ./data:/data:ro --mount ./cache:/cache:rw

# workspaceを読み取り専用、出力先を読み書き可能にする
codex-dock run --read-only --mount ./output:/output:rw
```

`--read-only` は `/workspace` に適用します。追加マウントはそれぞれ `ro` / `rw` で指定します。
読み取り専用でも、ホスト側で行った変更はVMから読み取れます。
VMからの作成・上書き・削除・chmodは拒否します。

相対パスはCLIを実行したディレクトリから解決します。
必要なディレクトリやファイルは先に用意してください。
単一ファイルのマウントは、ホストで同じパスを別inodeへ置換した場合、古いファイルを参照し続ける場合があります。
エディターの原子的な保存を反映させたい場合は、親ディレクトリを共有してください。

## 停止・再起動・削除

停止・再起動で、共有元のホストファイルをコピーし直すことはありません。
VM内のイメージ上のファイルと、ホストから共有したファイルは別です。
通常のプロジェクト共有はワーカー削除後もホストに残ります。
CLIが作成したworktreeは、ワーカーの削除時に回収します。成果は先に保存してください。

## 検証する

準備済みLinux/KVMノードで `make kata-e2e` を実行します。
`TestKataWorkspaceSharing` は、実CLIから起動し、ワーカー本来のentrypointと権限でファイル操作を行います。
管理者用execによる代替試験ではありません。

確認項目は、双方向の作成・編集・置換・名前変更・削除、開いたままのファイル、数値所有者、パーミッション、別UIDの0600ファイル、setgid、読み取り専用、追加マウント、相対パス、worktree、再起動です。
変更監視はVM内のイベントを対照にして、ホスト由来のinotify通知とNodeのポーリングを別々に確認します。
動作確認の結果は[テストと動作確認](testing.md)に記録します。
