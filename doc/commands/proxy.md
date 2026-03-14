# `codex-dock proxy` — Auth Proxy の管理

> **日本語** | [English](../../en/commands/proxy.md)
>
> [← コマンドリファレンス一覧](../commands.md)

Auth Proxy コンテナのビルド・起動・停止・削除を行います。
`proxy run` は内部的に `docker compose up -d` を使用して起動します。

---

## サブコマンド一覧

| サブコマンド | 説明 |
|---|---|
| [`proxy build`](#proxy-build--auth-proxy-イメージのビルド) | Auth Proxy イメージをビルド |
| [`proxy run`](#proxy-run--auth-proxy-コンテナの起動) | Auth Proxy コンテナを起動 |
| [`proxy serve`](#proxy-serve--ローカルプロセスとして起動) | ローカルプロセスとして起動 |
| [`proxy stop`](#proxy-stop--auth-proxy-コンテナの停止) | Auth Proxy コンテナを停止 |
| [`proxy rm`](#proxy-rm--auth-proxy-コンテナの削除) | Auth Proxy コンテナを削除 |

---

## `proxy build` — Auth Proxy イメージのビルド

```bash
codex-dock proxy build [OPTIONS]
```

| オプション | 省略形 | デフォルト | 説明 |
|---|---|---|---|
| `--tag` | `-t` | `proxy_image`（config） | イメージのタグ |
| `--dockerfile` | `-f` | （自動検出） | auth-proxy.Dockerfile のパス |

### Dockerfile の検索順序

`-f` を省略した場合、以下の順序で自動検出します：

1. カレントディレクトリの `auth-proxy.Dockerfile`
2. カレントディレクトリの `docker/auth-proxy.Dockerfile`
3. `~/.config/codex-dock/auth-proxy.Dockerfile`（存在しない場合は組み込みデフォルトを自動書き出し）

> ビルドコンテキストは常にカレントディレクトリ（`.`）です。Auth Proxy イメージは Go ソースから
> バイナリをコンパイルするため、リポジトリルートで実行してください。

**使用例：**

```bash
# デフォルト設定でビルド
codex-dock proxy build

# カスタムタグでビルド
codex-dock proxy build --tag my-proxy:v1
```

---

## `proxy run` — Auth Proxy コンテナの起動

```bash
codex-dock proxy run [OPTIONS]
```

| オプション | 省略形 | デフォルト | 説明 |
|---|---|---|---|
| `--name` | | `codex-auth-proxy` | コンテナ名 |
| `--port` | `-p` | `18080` | ホスト側のポート番号 |
| `--network` | | `dock-net-proxy` | 接続先 Docker ネットワーク（存在しない場合は自動作成） |
| `--admin-secret` | | | `/admin/*` エンドポイントの認証シークレット |

> 生成される compose 内容は、`examples/proxy-standalone/docker-compose.yml` と同等の構成です。

### 認証情報の自動バインド

`proxy run` は以下のすべての認証ソースを自動的に検出してコンテナへバインドします：

| 認証方式 | ホスト側のソース | コンテナへの渡し方 |
|---|---|---|
| API キー（環境変数） | `OPENAI_API_KEY` | `-e OPENAI_API_KEY=<値>` |
| API キー（保存済み） | `~/.config/codex-dock/apikey` | bind-mount（読み取り専用） |
| OAuth / ChatGPT | `~/.codex/auth.json` | bind-mount（読み取り専用） |

存在するすべてのソースが同時にバインドされます。優先順位は `OPENAI_API_KEY` 環境変数 → 保存済みキーファイル → OAuth の順です。

**使用例：**

```bash
# デフォルト設定で起動（認証情報は自動検出）
codex-dock proxy run

# ポートとシークレットを指定
codex-dock proxy run --port 9090 --admin-secret mysecret
```

---

## `proxy serve` — ローカルプロセスとして起動

Auth Proxy をローカルプロセス（Docker コンテナではなく）として起動します。

```bash
codex-dock proxy serve --listen 0.0.0.0:18080 [OPTIONS]
```

| オプション | デフォルト | 説明 |
|---|---|---|
| `--listen` | `127.0.0.1:18080` | リッスンアドレス |
| `--admin-secret` | | `/admin/*` エンドポイントの認証シークレット |

> この場合、ワーカーコンテナから到達できる URL を `--proxy-container-url` で指定してください。
> Auth Proxy のみを使う場合の詳細は [Auth Proxy のみを使う](../proxy-standalone.md) を参照してください。

---

## `proxy stop` — Auth Proxy コンテナの停止

```bash
codex-dock proxy stop [OPTIONS]
```

| オプション | 省略形 | デフォルト | 説明 |
|---|---|---|---|
| `--name` | | `codex-auth-proxy` | コンテナ名 |

---

## `proxy rm` — Auth Proxy コンテナの削除

```bash
codex-dock proxy rm [OPTIONS]
```

| オプション | 省略形 | デフォルト | 説明 |
|---|---|---|---|
| `--name` | | `codex-auth-proxy` | コンテナ名 |
| `--force` | `-f` | `false` | 実行中のコンテナも強制削除 |

---

## 関連ドキュメント

- [Auth Proxy 技術仕様](../auth-proxy.md) — 認証の仕組みと構成図
- [Auth Proxy エンドポイント仕様](../auth-proxy/endpoints.md) — 管理 API の詳細
- [Auth Proxy のみを使う](../proxy-standalone.md) — codex-dock run を使わない設定方法
- [クイックスタート](../getting-started.md) — Auth Proxy の初回セットアップ手順
