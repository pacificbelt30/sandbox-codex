# 日常の運用

| 目的 | このページの操作・参照先 |
|---|---|
| 初回準備 | [クイックスタート](getting-started.md) |
| ホストの配置 | [Kataの手順](kata.md) |
| 自動配置環境の更新・復旧 | [setup](setup.md#更新する) |
| 手動配置環境のCLI・Proxy更新 | [更新手順](#cliプロキシを更新する) |

## 状態とログを見る

```bash
oaiso ps -a
oaiso vm ps -a
oaiso vm inspect WORKER
oaiso logs WORKER --follow
oaiso ui
```

| 操作 | 確認できるもの |
|---|---|
| `ps` | ワーカーの状態 |
| `vm ps` / `vm inspect` | containerd・予約台帳・ホストプロセスを照合したVMの状態 |
| `logs --follow` | ログの追跡 |
| `ui` | TUIでの一覧・管理 |

- VM稼働の確認と、アプリケーションの正常応答の確認は別。
- 状態の意味：[VM管理](vm.md)。

## 停止・再起動・削除する

```bash
oaiso stop WORKER
oaiso start WORKER
oaiso stop WORKER
oaiso rm WORKER
```

| 操作 | 認証・保存状態 |
|---|---|
| 停止 | 認証を失効。コンテナとsnapshotを保持 |
| 再起動 | Kataを再検証。同じID・workspace・placeholderを復元 |
| 停止済みワーカーの削除 | `rm WORKER`。必要な成果は先に保存 |
| 実行中ワーカーの削除 | `rm --force WORKER` |
| 削除失敗 | 同じコマンドで回収を再試行 |

## 並列で実行する

```bash
oaiso run --agent codex --parallel 2 --worktree \
  --task "テストを追加して" --approval-mode full-auto --detach
```

| 項目 | 動作・設定 |
|---|---|
| 分離 | ワーカーごとに別VM・別worktree |
| ノードの既定上限 | 同時2台 |
| 残存資源の確認 | `vm ps -a` |
| 上限の変更 | 管理者が `node serve --max-workers` で設定 |
| CPU・メモリの要件 | [必要なホスト](kata.md#必要なホスト) |

## CLI・プロキシを更新する

- 自動配置した環境：[setupによる更新・復旧](setup.md#更新する)。
- 以下の手順：手動配置の更新用。
- 認証方式・通信ポリシーを変える場合も同じ順序。
- 共有Proxyを更新する前に、対象ワーカーの作業を終了。

### 更新手順

1. `ps -a` で対象を確認し、ログと成果を保存。
2. 旧版CLIでワーカーを停止・削除。
3. CLIと管理サービスを同じ版へ更新。
4. リポジトリのルートで以下を実行。
5. 更新後は新規ワーカーを作成。

```bash
oaiso proxy stop
oaiso proxy rm
oaiso proxy build
oaiso proxy run
oaiso runtime-check
```

| 条件・操作 | 注意点 |
|---|---|
| 保存済みワーカー | Proxy IPが変わると再起動できないため、新規作成 |
| ソース配置 | `make build` 後、[配置手順](kata.md)のバイナリを更新 |
| `proxy stop` / `proxy rm` の名前省略 | 既定の2つのProxyが対象 |
| 独自Proxy名 | それぞれ `--name NAME` で指定 |
| `proxy build` のみ実行 | 既存コンテナは更新されない |
| 旧Proxy | `worker_token_auth: true` 非対応なら新CLIは拒否 |
| 新規ワーカーの起動 | 新しいイメージからProxyを再作成した後 |

### 更新対象を確認する

| 対象 | 必要な作業 |
|---|---|
| [Go API](go-api.md)の組み込み製品 | 受付を停止 → Goパッケージも同じソース版へ更新 → 再接続確認 |
| nodeの版が非対応 | `Open` が `ErrVersionMismatch` を返す |
| shim・kernel・guest imageなど | 別途[Kataの固定資産更新](kata.md#固定資産の更新)を実施 |
| `oaiso update` | Goのインストール先のCLIのみ更新。サービス・イメージは対象外 |

## ワーカー内のCodex CLIを更新する

### 版固定と互換性

| 項目 | 扱い |
|---|---|
| ホストの `codex` | 現在は主に認証ファイル作成に使用。ワーカーCLIと直接通信しない |
| ホスト／ワーカーの版番号 | 常に一致させる必要はない |
| 互換性の確認点 | ホスト認証ファイル形式・ワーカー設定キー・Proxy転送経路 |
| 組み込みイメージのCLI | 固定版。ゲスト起動時の更新確認も無効 |
| 実行中のCLI | npmで書き換えない |

### 更新手順

1. 別タグで候補イメージを作成：`CODEX_DOCK_CODEX_VERSION=<版> oaiso build --template plain --tag codex-dock:codex-<版>`。
2. 基本要件を検査：`oaiso image check codex-dock:codex-<版> --agent codex`。
3. 検証ホストで起動：`oaiso run --image codex-dock:codex-<版> --agent codex`。
4. APIキー・ChatGPT認証で、実際の対話・SSE・OAuth更新を確認。
5. 成功した場合だけ、新規ワーカーの `default_image` を候補タグへ変更。

- `image check` だけでは、認証の互換性を確認できない。
- 既存ワーカー：古いイメージのまま完了させる。
- 失敗時：以前のタグへ戻す。

### 自動追従の範囲

| 項目 | 現状・方針 |
|---|---|
| 新版検出・自動昇格 | 未実装 |
| 将来の流れ | npm新版検出 → 候補タグ作成 → 実環境ゲート → 新規ワーカー向け既定タグ変更 |
| Proxyの基本中継 | `/v1/*`・`/chatgpt/*` のパス・クエリ・本文を転送 |
| 認証の処理 | 上流認証ヘッダーの置換・OAuth更新のrefresh token秘匿 |
| CLIのベースURL・認証方式変更 | 設定と接続試験の更新が必要 |

## 後片付けする

**ワーカーを削除した後に、共有Proxyとegressネットワークを停止します。**

```bash
oaiso proxy stop
oaiso proxy rm
oaiso network rm
```

- 孤立予約の確認・回収：[VM管理](vm.md#孤立した資源の回収)。
- `vm prune`：空の孤立予約が対象。
- 停止済みワーカーは削除しない。
