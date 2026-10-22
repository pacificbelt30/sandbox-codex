# トラブルシューティング

- 通常の利用者で状態を確認し、ホストサービスや固定資産の修正は管理者が行います。
- 認証ファイルの本文やトークンをログへ貼り付ける必要はありません。

## 管理ソケットに接続できない

```bash
id
ls -l /run/codex-dock-node.sock
systemctl status codex-dock-containerd codex-dock-node
```

- 利用者が `codex-dock` グループに属しているか確認します。
- グループへ追加した直後は、再ログインして反映してください。
- 独自パスを使う場合は `CODEX_DOCK_KATA_NODE_SOCKET` とサービスの設定を合わせます。

## Kataの起動検証に失敗する

```bash
oaiso runtime-check
oaiso vm ps -a
journalctl -u codex-dock-node -u codex-dock-containerd --since "10 minutes ago"
```

- エラーに示されたKVM、cgroup、manifest、固定資産の所有権・checksumを確認します。
- 本製品は検証を省略して起動したり、自動でruncへ切り替えたりしません。
- 資産を更新した場合は[Kataの更新手順](kata.md#固定資産の更新)を参照してください。

## 認証プロキシに接続できない

```bash
oaiso proxy run
oaiso logs codex-auth-proxy
```

- 管理APIの既定URLは `http://127.0.0.1:18081` です。
- `connection refused` は、プロキシ未起動や接続先の不一致を確認します。
- 管理secretを設定している場合は、CLIとProxyの両方を同じ `[proxy].admin_secret_file` へ合わせます。所有者0600・実際の絶対パスを確認。
- 値指定とファイル指定が同時に有効ならエラー。古い `proxy.admin_secret` と `CODEX_DOCK_PROXY_ADMIN_SECRET` も確認し、片方の方式へ統一。
- 独立プロファイルは[選択確認](guides/authentication.md)を行い、`init` に登録したproviderを指定。

- `worker_token_auth` 非対応のエラーが出る場合は旧プロキシです。
- [停止・削除・再ビルド・再作成](operations.md#cliプロキシを更新する)を行ってください。

## auth.jsonを読み取れない

```bash
id
ls -l ~/.codex/auth.json
```

- 認証ファイルの所有者と、`proxy run` を実行する利用者を合わせます。
- 認証プロキシは呼出元のUID/GIDで読み取り専用マウントを開きます。
- `sudo` で別のホームを参照していないかも確認してください。
- 所有者のファイルは `0600` のまま使えます。

## AI APIが401を返す

- プロキシが返す `worker token invalid or expired` は、短命トークンの欠落・失効・期限切れを示します。
- 停止後の再起動には `oaiso start WORKER` を使い、認証を復元します。
- TTLを超えるタスクでは `run --token-ttl SECONDS` を調整してください。

- 上流API由来の認証エラーは、ホストのログイン状態やAPIキーを確認します。
- 認証ソースを変更した場合はプロキシの再作成が必要です。

## 一般通信ができない

```bash
oaiso network status
oaiso logs codex-http-proxy
```

- `--no-internet` の有無と[通信ポリシー](proxy-policy.md)を確認します。
- 未設定時も一般通信は既定拒否。ドメイン・CIDRの許可が空なら、`--no-internet` なしでもegressを接続しません。
- [宛先許可 → 検証 → Proxy・ワーカー再作成](guides/networking.md)の順で設定します。
- このオプションでは一般egressを接続せず、AI API経路だけを維持します。
- private IP、loopback、link-local、metadata宛ては既定で拒否します。

- 管理者はホストのIPv4 forwarding、既存firewallのforward方針、プロキシから到達可能なDNSを確認します。
- Dockerが同居するホストでは、既存の `FORWARD DROP` が通信を遮断する場合もあります。
- 全体のルールをflushせず、実行ホストの方針に沿って必要な経路だけを許可してください。
- loopback専用DNSはプロキシの別namespaceから使えません。
- 必要な場合は `node serve --proxy-dns ADDRESS` を指定します。

## イメージをビルドできない

```bash
systemctl status codex-dock-buildkit
command -v buildctl
```

- BuildKitのソケットと `BUILDKIT_HOST`、利用者のソケット権限を確認します。
- プロキシイメージはGoソースが必要なので、リポジトリのルートからビルドします。
- 外向きproxyやCAが必要な環境では、CLIのbuild argsとBuildKit daemonのregistry通信の両方を設定してください。
- [ビルドの詳細](commands/build.md)を参照してください。

## VMや削除途中の資源が残っている

```bash
oaiso vm ps -a
oaiso vm inspect WORKER
oaiso rm WORKER
```

- containerdに接続できない場合は `unknown` と表示し、孤立資源として削除しません。
- 管理層を復旧してから、[VM管理](vm.md)の回収手順を使います。
- 台帳やcgroupを手動で削除する前に、実プロセスと所有関係を確認してください。

## 回収記録の保存先を拒否される

- 回収記録は、通常は `~/.codex-dock-state` 配下へ保存します。
- `XDG_STATE_HOME` や `CODEX_DOCK_STATE_DIR` を指定している場合は、そのパスも確認してください。
- 保存先と親ディレクトリの所有者・権限を確認し、グループ／他ユーザーが書き込める場所やsymlinkを避けます。

- 起動前に、所有関係だけでなく実際の書込みとfsyncも確認します。
- この確認で失敗した場合、ワーカーやネットワークは作成しません。

## 旧ノードの共有cache対応を拒否される

- 新しいCLIは、管理サービスの `SharedFSCache` が `none` であることも確認します。
- 古いnodeはこの能力を返さないため接続を拒否します。
- Kata設定・manifestとともにnodeを同じ版へ更新してください。

## ホストの編集がVMに反映されない

- 現行構成は `virtio_fs_cache = "none"` と、`virtio_fs_extra_args` の `open,no_writeback_cache` を要求します。
- cacheモードだけを変更しても、開いたままのファイルではwriteback cacheの影響が残ります。
- 古い `auto` / `always` 設定では、ホストの上書きがVM側に古い内容として残る場合があります。
- [Kataの固定資産更新](kata.md#固定資産の更新)に沿って設定とmanifestを再生成し、新しいワーカーで確認してください。

- `--worktree` を使っている場合は、元のcheckoutではなく、作成したworktreeが共有先です。
- 単一ファイルをマウントしている場合は、ホストの原子的な置換後も元inodeを参照する場合があります。
- その用途では[親ディレクトリを共有](workspace.md#読み取り専用と追加マウント)してください。

## ホストで保存しても開発サーバーが再読み込みしない

- VMでファイルを読み直した内容が最新なら、変更通知の方式を確認してください。
- 実機試験では、ホストの変更はVM内のinotifyへ届きません。
- 監視ツールのポーリングを有効にします。
- [変更監視の例](workspace.md#ホスト側の変更を監視する)を参照してください。

## 複数の前提条件をまとめて調べる

```bash
oaiso doctor
oaiso runtime-check --json
```

**正常時**

- [クイックスタートの出力例](getting-started.md#1-実行ホストを確認する)と同様に、全項目 `[ok]`・終了コード `0`。
- JSONの成功判定：`checks` の全項目が `"status": "ok"`。

**KVMデバイスがない場合の該当箇所（他の診断項目は省略）**

```text
[error] kvm: stat /dev/kvm: no such file or directory
  Fix: Enable CPU virtualization/nested virtualization, then sudo modprobe kvm_intel (Intel) or kvm_amd (AMD).
```

| 表示・挙動 | 次の操作 |
|---|---|
| `[error]` と `Fix:` | 該当項目を修正して診断を再実行 |
| 最後の `readiness checks failed: ...` | 失敗した項目名の一覧。各項目の詳細へ戻る |
| JSONで `"status": "error"` | `detail` と `fix` を確認。JSONを取得できても終了コードは非ゼロ |

- KVM、cgroup、管理サービスの固定資産検証、BuildKit、IPv4 forwardingを個別に表示します。
- 失敗した項目には確認・修正の手順を付け、他の項目の確認も続けます。
- JSON出力でも失敗時の終了コードは非ゼロです。
- 診断は設定を変更しません。
- KVMのCLI利用者向け権限は不要です。
- VMはroot管理サービスが起動します。
