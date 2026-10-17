# トラブルシューティング

通常の利用者で状態を確認し、ホストサービスや固定資産の修正は管理者が行います。
認証ファイルの本文やトークンをログへ貼り付ける必要はありません。

## 管理ソケットに接続できない

```bash
id
ls -l /run/codex-dock-node.sock
systemctl status codex-dock-containerd codex-dock-node
```

利用者が `codex-dock` グループに属しているか確認します。
グループへ追加した直後は、再ログインして反映してください。
独自パスを使う場合は `CODEX_DOCK_KATA_NODE_SOCKET` とサービスの設定を合わせます。

## Kataの起動検証に失敗する

```bash
codex-dock runtime-check
codex-dock vm ps -a
journalctl -u codex-dock-node -u codex-dock-containerd --since "10 minutes ago"
```

エラーに示されたKVM、cgroup、manifest、固定資産の所有権・checksumを確認します。
本製品は検証を省略して起動したり、自動でruncへ切り替えたりしません。
資産を更新した場合は[Kataの更新手順](kata.md#固定資産の更新)を参照してください。

## 認証プロキシに接続できない

```bash
codex-dock proxy run
codex-dock logs codex-auth-proxy
```

管理APIの既定URLは `http://127.0.0.1:18081` です。
`connection refused` は、プロキシ未起動や接続先の不一致を確認します。
管理secretを設定している場合は、CLI側の `proxy.admin_secret` と合わせます。

`worker_token_auth` 非対応のエラーが出る場合は旧プロキシです。
[停止・削除・再ビルド・再作成](operations.md#cliプロキシを更新する)を行ってください。

## auth.jsonを読み取れない

```bash
id
ls -l ~/.codex/auth.json
```

認証ファイルの所有者と、`proxy run` を実行する利用者を合わせます。
認証プロキシは呼出元のUID/GIDで読み取り専用マウントを開きます。
`sudo` で別のホームを参照していないかも確認してください。
所有者のファイルは `0600` のまま使えます。

## AI APIが401を返す

プロキシが返す `worker token invalid or expired` は、短命トークンの欠落・失効・期限切れを示します。
停止後の再起動には `codex-dock vm start WORKER` を使い、認証を復元します。
TTLを超えるタスクでは `run --token-ttl SECONDS` を調整してください。

上流API由来の認証エラーは、ホストのログイン状態やAPIキーを確認します。
認証ソースを変更した場合はプロキシの再作成が必要です。

## 一般通信ができない

```bash
codex-dock network status
codex-dock logs codex-http-proxy
```

`--no-internet` の有無と[通信ポリシー](proxy-policy.md)を確認します。
このオプションでは一般egressを接続せず、AI API経路だけを維持します。
private IP、loopback、link-local、metadata宛ては既定で拒否します。

管理者はホストのIPv4 forwarding、既存firewallのforward方針、プロキシから到達可能なDNSを確認します。
Dockerが同居するホストでは、既存の `FORWARD DROP` が通信を遮断する場合もあります。
全体のルールをflushせず、実行ホストの方針に沿って必要な経路だけを許可してください。
loopback専用DNSはプロキシの別namespaceから使えません。
必要な場合は `node serve --proxy-dns ADDRESS` を指定します。

## イメージをビルドできない

```bash
systemctl status codex-dock-buildkit
command -v buildctl
```

BuildKitのソケットと `BUILDKIT_HOST`、利用者のソケット権限を確認します。
プロキシイメージはGoソースが必要なので、リポジトリのルートからビルドします。
外向きproxyやCAが必要な環境では、CLIのbuild argsとBuildKit daemonのregistry通信の両方を設定してください。
[ビルドの詳細](commands/build.md)を参照してください。

## VMや削除途中の資源が残っている

```bash
codex-dock vm ps -a
codex-dock vm inspect WORKER
codex-dock rm WORKER
```

containerdに接続できない場合は `unknown` と表示し、孤立資源として削除しません。
管理層を復旧してから、[VM管理](vm.md)の回収手順を使います。
台帳やcgroupを手動で削除する前に、実プロセスと所有関係を確認してください。
