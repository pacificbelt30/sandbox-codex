# 日常の運用

準備と初回起動は[クイックスタート](getting-started.md)、ホストの配置は[Kataの手順](kata.md)を参照してください。

## 状態とログを見る

```bash
cxd ps -a
cxd vm ps -a
cxd vm inspect WORKER
cxd logs WORKER --follow
cxd ui
```

`ps` はワーカー、`vm ps` はcontainerd・予約台帳・ホストプロセスを照合したVMを表示します。
VMが動いていてもアプリケーションの正常応答までは保証しません。
状態の意味は[VM管理](vm.md)を参照してください。

## 停止・再起動・削除する

```bash
cxd stop WORKER
cxd start WORKER
cxd stop WORKER
cxd rm WORKER
```

停止中は認証が失効し、コンテナとsnapshotは再起動用に残ります。
再起動時はKataの検証をやり直し、同じID・workspace・プレースホルダートークンを復元します。

実行中のワーカーを削除する場合は `rm --force WORKER` を使います。
削除に失敗した場合は同じコマンドで回収を再試行できます。
必要な成果を保存してから削除してください。

## 並列で実行する

```bash
cxd run --agent codex --parallel 2 --worktree \
  --task "テストを追加して" --approval-mode full-auto --detach
```

各ワーカーに別のVMとworktreeを用意します。
ノードの既定上限は同時2台です。残存資源は `vm ps -a` で確認します。
上限の変更は管理者が `node serve --max-workers` で行います。
CPUとメモリの要件は[Kataの手順](kata.md#必要なホスト)を参照してください。

自動セットアップで配置した環境は、[setupによる更新・復旧](setup.md#更新する)を使えます。
以下は手動配置の更新手順です。

## CLI・プロキシを更新する

認証方式や通信ポリシーを変える場合も、この順序で更新します。
プロキシは複数ワーカーで共有しているため、先にワーカーの作業を終えます。

1. `ps -a` で対象を確認し、ログと成果を保存する。
2. 旧版でワーカーを停止・削除する。プロキシIPが変わると保存済みワーカーを再起動できないため、更新後は新規作成する。
3. CLIと管理サービスを同じ版に更新する。ソース配置の場合は `make build` の後、[配置手順](kata.md)のバイナリを更新する。
4. リポジトリのルートで、次を実行する。

```bash
cxd proxy stop
cxd proxy rm
cxd proxy build
cxd proxy run
cxd runtime-check
```

名前を省略した `proxy stop` と `proxy rm` は、既定の2つのプロキシが対象です。
独自名を使った場合は、それぞれ `--name NAME` で指定します。

`proxy build` だけでは既存コンテナは更新されません。
新しいCLIは `worker_token_auth: true` を返さない旧プロキシを拒否します。
新しいイメージからプロキシを作り直してからワーカーを起動してください。

Kataのshim・kernel・guest imageなどの固定資産を更新する場合は、別途[Kataの資産更新](kata.md#固定資産の更新)が必要です。
`cxd update` はGoのインストール先にCLIを置く操作だけで、ホストサービスやイメージを更新しません。

## ワーカー内のCodex CLIを更新する

ホストの `codex` は現在、主に認証ファイルを作る役割で、ワーカー内のCodex CLIとは直接通信しません。そのため両者の版番号を常に一致させる必要はありません。互換性の境界は、ホストの認証ファイル形式、ワーカーの設定キー、認証プロキシの転送経路です。

組み込みイメージのCodex CLIは版を固定しています。ゲストの起動時更新確認も無効にしており、実行中にnpmで書き換えません。更新は次の順で行います。

1. 新版を `CODEX_DOCK_CODEX_VERSION=<版> cxd build --template plain --tag codex-dock:codex-<版>` で別タグにビルドする。
2. `cxd image check codex-dock:codex-<版> --agent codex` で実行ファイルとイメージの基本要件を確認する。
3. 検証用ホストで `cxd run --image codex-dock:codex-<版> --agent codex` を起動し、APIキーとChatGPT認証で実際の対話・SSE・OAuth更新を確認する。`image check` だけでは認証の互換性を確認できない。
4. 問題がなければ新規ワーカーの `default_image` を候補タグに切り替える。既存ワーカーは古いイメージのまま完了させる。失敗したら以前のタグに戻す。

将来の自動更新は、定期ジョブでnpmの新版を検出して候補タグを作り、上記の実環境ゲートを通過した後に新規ワーカー向け既定タグだけを切り替える形を推奨します。このリポジトリでは候補の自動検出・自動昇格はまだ行いません。

認証プロキシは `/v1/*` と `/chatgpt/*` のパス・クエリ・本文をAPIごとに解釈せず転送します。上流認証ヘッダーの置換とOAuth更新時のrefresh token秘匿だけを担います。新しいCodexが別のベースURLや認証方法を使う場合は、設定と接続テストの更新が必要です。

## 後片付けする

ワーカーを削除した後、プロキシと共通egressネットワークを止める場合は次を実行します。

```bash
cxd proxy stop
cxd proxy rm
cxd network rm
```

孤立したVM予約は[VM管理の回収手順](vm.md#孤立した資源の回収)で確認します。
`vm prune` は空の孤立予約を対象とし、停止済みワーカーは削除しません。
