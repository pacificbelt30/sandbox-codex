# クイックスタート

**日本語** | [English](en/getting-started.md)

準備済みの実行ホストで、最初のワーカーを起動する手順です。
ホストをまだ用意していない場合は、管理者が先に[Kata / Dragonballを配置](kata.md)してください。

## 1. 実行ホストを確認する

operator groupに属する利用者で実行します。通常の `run` や `proxy run` に `sudo` は使いません。

```bash
cxd runtime-check
```

Linux x86_64、KVM、cgroup v2、専用containerd、Kata runtime-rs、Dragonballが必要です。
イメージの作成にはBuildKitと `buildctl` も必要です。
エラーが出た場合は[トラブルシューティング](troubleshooting.md)を参照してください。

ソースからCLIを用意する場合は、リポジトリで次を実行します。
Go 1.24.7以上とMakeが必要です。管理サービスや固定資産の配置は別途必要です。

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

既存のログインファイルは、`cxd` を実行する利用者のホームに置きます。
APIキーを使う場合は環境変数を設定します。OpenAIのキーは `cxd auth set` で保存することもできます。
[認証の優先順位](auth-proxy.md#認証ソース)も確認してください。

認証プロキシは同じ利用者のUID/GIDでファイルを読み取ります。
所有者だけが読める `0600` のまま利用でき、元ファイルの権限を広げる必要はありません。
実APIキーやOAuthのaccess token・refresh tokenはワーカーに渡しません。

## 3. プロキシとイメージを用意する

ホストの準備と認証設定を終えたら、通常利用者で次を実行できます。

```bash
cxd init
# Claude Codeを使う場合
cxd init --agent claude
```

診断、認証ソースの存在確認、必要なワーカーイメージのビルド、イメージ互換性検査、プロキシ起動を順に行います。
認証の本文は表示せず、既存設定は書き換えません。認証確認は上流APIへの疎通・推論試験ではありません。
既存プロキシの認証方式が違い、ワーカーが稼働中の場合は強制変更せず停止します。
プロキシイメージがない場合はGoソースから作成するため、リポジトリのルートで実行してください。
別のプロジェクトから `init` を実行してソースが見つからない場合は、リポジトリのルートで一度 `cxd proxy build` を実行してから戻ってください。
準備が終わったら、プロジェクトへ移動して `cxd run --agent codex` を実行します。

手動で分けて準備する場合は、次の手順を使います。


プロキシのビルドは、このリポジトリのルートで実行してください。
プロキシイメージはGoソースから作成します。

```bash
cxd proxy build
cxd proxy run
cxd build
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
[ファイル共有と所有者](workspace.md)で、双方向の反映とUID/GIDの扱いを説明しています。

チームで同じワーカー環境を使う場合は、プロジェクトのルートで `cxd project init` を実行し、生成された `codex-dock.toml` をコミットします。独自のツールが必要なら、プロジェクトのDockerfileを `[build]` から指定します。クローン先ではホストの準備と認証を済ませてから `cxd project check`、`cxd init` を実行してください。[設定例](configuration.md#プロジェクト設定)を参照してください。

```bash
cxd codex
# または
cxd run --agent claude
```

`--agent` を省略すると、利用可能な認証を設定したシェルが開きます。
コマンドを1回だけ実行することもできます。

```bash
cxd run
cxd run -- ls -la
```

Kataでは、独立したカーネルと権限・資源の制限を確認してから処理を始めます。
確認に失敗した場合は起動を中止します。

## 5. バックグラウンドで実行する

```bash
cxd run --agent codex --name add-tests \
  --task "ユニットテストを追加して" --approval-mode full-auto \
  --worktree --branch add-tests --new-branch -d

cxd ps -a
cxd logs add-tests --follow
```

`--approval-mode full-auto` はエージェントの承認確認を省略します。
共有ファイルへの書き込み範囲は、[マウントと承認設定](commands/run.md)で確認してください。

## 6. 停止・再起動・削除する

```bash
cxd stop add-tests
cxd start add-tests
cxd rm add-tests --force
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
