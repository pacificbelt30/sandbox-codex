# イメージの作成

**日本語** | [English](../en/commands/build.md)

`codex-dock build` はBuildKitでOCIイメージを作り、ワーカーと同じcontainerd namespaceへ取り込みます。
Dockerfile形式を使いますが、Docker EngineとDocker CLIは不要です。

## 基本操作

```bash
codex-dock build
codex-dock build --template list
codex-dock build --template pwn
codex-dock build -f ./Dockerfile --tag my-codex:v1
codex-dock run --image my-codex:v1
```

| オプション | 内容 |
|---|---|
| `--tag`, `-t` | 出力タグ。通常は `codex-dock:latest`、テンプレート指定時はそのタグ |
| `--dockerfile`, `-f` | Dockerfileを指定 |
| `--template`, `-T` | テンプレート名。`list` で一覧表示 |
| `--print-entrypoint` | 組み込みentrypoint.shを出力し、ビルドせず終了 |

組み込みワーカーのCodex CLIはDockerfile内の `CODEX_CLI_VERSION` に固定しています。
別の版を試す場合は、そのビルドだけ `CODEX_DOCK_CODEX_VERSION` を指定します。

```bash
CODEX_DOCK_CODEX_VERSION=0.163.0 codex-dock build --template plain --tag codex-dock:codex-0.163.0
codex-dock image check codex-dock:codex-0.163.0 --agent codex
codex-dock run --image codex-dock:codex-0.163.0 --agent codex
```

BuildKitで作成したOCIイメージはワーカーと同じ専用containerd namespaceに取り込まれます。候補タグを使って試せるため、現在使っているイメージはそのまま残せます。

`--template` と `-f` は同時に指定できません。
`run` 時にイメージがない場合は、対応するテンプレートまたは既定Dockerfileから自動ビルドします。
任意のイメージ名を指定する場合は、先にそのタグをビルドしてください。

## カスタムイメージを作る

組み込みイメージから派生すると、エージェントと認証用entrypointを引き継げます。

```bash
codex-dock build
```

新しい作業ディレクトリに `Dockerfile` を作成します。

```dockerfile
FROM codex-dock:latest
USER root
RUN apt-get update \
    && apt-get install -y --no-install-recommends jq \
    && rm -rf /var/lib/apt/lists/*
USER codex
```

そのディレクトリでビルドします。

```bash
codex-dock build -f ./Dockerfile --tag my-codex:jq
codex-dock run --image my-codex:jq -- jq --version
```

ビルド時の `USER root` はパッケージ追加用です。
Kataワーカーの実行ユーザーは非rootに解決し、capability・seccomp・資源制限を検証します。
イメージに認証情報や管理ソケットを含めないでください。

## 独自ベースイメージを使う

組み込みイメージを継承しない場合は、使用するエージェントとentrypointの依存ツールを用意します。
`docker/sandbox/Dockerfile` を基準にしてください。
通常の認証付き起動には、対応するentrypointが必要です。

```bash
codex-dock build --print-entrypoint > entrypoint.sh
```

Dockerfileに組み込みます。

```dockerfile
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
USER codex
WORKDIR /workspace
ENTRYPOINT ["/entrypoint.sh"]
```

上の例は、必要なツールと非rootユーザーを用意した後の設定です。
CLI更新時にはentrypointも再取得してください。
カスタムイメージも通常と同じKata起動検証を通ります。イメージごとの互換性は実行して確認します。

## ユーザーテンプレート

`~/.config/codex-dock/templates/<名前>/Dockerfile` に配置すると、`--template <名前>` で使えます。
同名の組み込みテンプレートより優先します。

```bash
mkdir -p ~/.config/codex-dock/templates/mytool
cp Dockerfile ~/.config/codex-dock/templates/mytool/Dockerfile
codex-dock build --template mytool
codex-dock run --template mytool
```

`plain` のタグは `codex-dock:latest`、他は `codex-dock:<名前>` です。
`make install-templates` は組み込みテンプレートを利用者のディレクトリへ配置します。既存ファイルは上書きしません。
ディスク上のDockerfile更新時の自動ビルドは[auto_build](../configuration.md#auto_build)で設定します。

## Dockerfileとビルドコンテキスト

`-f` を指定した場合、コンテキストはカレントディレクトリです。
省略した場合は次の順に探し、選んだDockerfileのディレクトリを使います。

1. `./Dockerfile`
2. `./docker/sandbox/Dockerfile`
3. `./docker/Dockerfile`（旧配置）
4. `~/.config/codex-dock/Dockerfile`（未配置なら組み込み版を配置）

テンプレートのビルドでは、必要なentrypointなどを含む一時コンテキストを作成します。
プロキシイメージのビルドは別の[proxy build](proxy.md)です。

## BuildKitの接続と通信

既定のbuilderは `unix:///run/codex-dock-buildkit/buildkitd.sock` です。
変更する場合は `BUILDKIT_HOST` を指定します。
BuildKitのcontainerd workerは、製品の専用socket・namespaceとそろえます。

HTTP/HTTPS/NO/ALL proxy環境変数と小文字版をbuild argsとして渡します。
`CODEX_PROXY_CERT` は `proxy_ca` secretとして渡し、組み込みDockerfileはRUN中だけ使います。
独自Dockerfileで必要な場合もsecretとして参照し、イメージへコピーしないでください。
BuildKit daemonのregistry通信は、管理者が別途proxyとCAを設定します。

## 起動前に互換性を調べる

```bash
codex-dock image check my-codex:v1 --agent codex
codex-dock image check my-codex:v1 --agent claude --json
```

管理サービスがイメージの読み取り専用snapshotを検査します。イメージ内のコードはホストで実行しません。
Linux amd64、bash・curl・python3・git、選択したagent、ENTRYPOINT/CMDの実行ファイルを確認します。
`--agent shell` が既定で、エージェントの実行ファイルは要求しません。
認証entrypointがCLIの組み込み版と違う場合は警告します。独自wrapperもあり得るため、この違いだけで拒否はしません。

不足があれば非ゼロ終了します。通過しても、共有ライブラリ、実行ユーザー、認証・通信の動作までは保証しません。
カスタムイメージは準備済みKataノードで `codex-dock run --image my-codex:v1 -- true` も実行してください。
