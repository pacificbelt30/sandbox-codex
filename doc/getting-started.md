# クイックスタート

**日本語** | [English](en/getting-started.md)

準備済みの実行ホストで、最初のワーカーを起動する手順です。
ホストをまだ用意していない場合は、管理者が先に[Kata / Dragonballを配置](kata.md)してください。

## 1. 実行ホストを確認する

operator groupに属する利用者で実行します。通常の `run` や `proxy run` に `sudo` は使いません。

```bash
codex-dock runtime-check
```

Linux x86_64、KVM、cgroup v2、専用containerd、Kata runtime-rs、Dragonballが必要です。
イメージの作成にはBuildKitと `buildctl` も必要です。
エラーが出た場合は[トラブルシューティング](troubleshooting.md)を参照してください。

ソースからCLIを用意する場合は、リポジトリで次を実行します。
Go 1.24.7以上が必要です。管理サービスや固定資産の配置は別途必要です。

```bash
make build
make install-config
make install-templates
```

設定とテンプレートが既にある場合は上書きしません。

## 2. ホストの認証を用意する

使用するエージェントの認証を、プロキシ起動前に用意します。

| エージェント | 利用できるホスト側の認証 |
|---|---|
| Codex | `OPENAI_API_KEY`、保存済みAPIキー、`~/.codex/auth.json` |
| Claude Code | `ANTHROPIC_API_KEY`、保存済みAPIキー、`~/.claude/.credentials.json` |

既存のログインファイルは、`codex-dock` を実行する利用者のホームに置きます。
APIキーを使う場合は環境変数を設定します。OpenAIのキーは `codex-dock auth set` で保存することもできます。
[認証の優先順位](auth-proxy.md#認証ソース)も確認してください。

認証プロキシは同じ利用者のUID/GIDでファイルを読み取ります。
所有者だけが読める `0600` のまま利用でき、元ファイルの権限を広げる必要はありません。
実APIキーやOAuthのaccess token・refresh tokenはワーカーに渡しません。

## 3. プロキシとイメージを用意する

プロキシのビルドは、このリポジトリのルートで実行してください。
プロキシイメージはGoソースから作成します。

```bash
codex-dock proxy build
codex-dock proxy run
codex-dock build
```

`proxy run` はネットワークを自動作成し、次の2つを起動します。

| プロキシ | 役割 |
|---|---|
| `codex-auth-proxy` | AI APIへの認証付き通信 |
| `codex-http-proxy` | Git、npm、pipなどの一般通信。実認証情報は持たない |

管理APIはホストの `127.0.0.1:18081` に公開します。
ワーカー側のAPIポートはホストへ公開しません。
詳しくは[認証プロキシ](auth-proxy.md)を参照してください。

## 4. ワーカーを起動する

作業するプロジェクトのディレクトリで実行します。
カレントディレクトリをVM内の `/workspace` に共有します。

```bash
codex-dock run --agent codex
# または
codex-dock run --agent claude
```

`--agent` を省略すると、利用可能な認証を設定したシェルが開きます。
コマンドを1回だけ実行することもできます。

```bash
codex-dock run
codex-dock run -- ls -la
```

Kataでは、独立したカーネルと権限・資源の制限を確認してから処理を始めます。
確認に失敗した場合は起動を中止します。

## 5. バックグラウンドで実行する

```bash
codex-dock run --agent codex --name add-tests \
  --task "ユニットテストを追加して" --approval-mode full-auto \
  --worktree --branch add-tests --new-branch --detach

codex-dock ps -a
codex-dock logs add-tests --follow
```

`--approval-mode full-auto` はエージェントの承認確認を省略します。
共有ファイルへの書き込み範囲は、[マウントと承認設定](commands/run.md)で確認してください。

## 6. 停止・再起動・削除する

```bash
codex-dock stop add-tests
codex-dock vm start add-tests
codex-dock rm add-tests --force
```

停止すると短命トークンが失効します。再起動では同じワーカーIDとworkspaceを使い、認証を復元します。
削除すると、管理対象のネットワークやworktreeも回収します。必要な成果は先に保存してください。

前面で実行したワーカーは、既定では終了時に削除します。
残す場合は `--keep` を指定します。
プロキシは共有サービスなので、ワーカー終了後も動き続けます。

## 次に読む

- [日常の運用](operations.md)：VM一覧、並列実行、更新、後片付け
- [カスタムイメージ](commands/build.md)：Dockerfileとテンプレート
- [通信ポリシー](proxy-policy.md)：許可リスト、拒否リスト、`--no-internet`
- [トラブルシューティング](troubleshooting.md)：起動・認証・通信の確認
