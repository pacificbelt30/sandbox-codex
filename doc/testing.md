# テストと動作確認

テストを「コードの確認」「実Kataでの確認」「実AIとの通信」に分けて説明します。
自動テストの成功だけで、外部APIや任意のカスタムイメージの動作を保証するものではありません。

## コードを確認する

Go 1.24.7以上を使います。race検査にはCコンパイラーも必要です。

```bash
go test ./...
make test
make vet
gofmt -l cmd internal pkg
```

`make test` はrace検査とcoverageを有効にします。
通常のプロキシテストはローカルのHTTP fixtureを使うため、実APIキーは不要です。

| 対象 | 主な確認 |
|---|---|
| 認証ファイル | 所有者UID/GID、0600、読み取り専用mount、読み取り不能時の拒否 |
| ワーカートークン | 全AI経路のHTTP/WSで欠落・未発行・期限切れ・失効・重複・不一致を拒否 |
| 接続の寿命 | 期限切れ・失効・再発行・restore・プロキシ停止時にWebSocketを閉じる |
| 認証注入 | OpenAI / AnthropicのAPIキー・OAuth、プレースホルダーの除去 |
| ストリーミング | SSEの逐次受信、WebSocketの転送 |
| 通信ポリシー | ドメイン・IP・ポート、DNS応答、redirect、CONNECTの判定 |
| 実行管理 | 資産・cgroup・所有権検証、起動失敗の回収、VM一覧、削除の再試行 |
| 製品組み込み用 Go API | 公開エラー分類と要求IDの入力検査、node台帳での競合・再送・応答消失時の照合と回収、通信許可の入力検査 |
| ケース別認証 | 資格情報ソケットの権限とケース分離、OAuth更新の版競合、ケース変更時の失効、プロバイダー経路と実行IDの失効 |

[Go API](go-api.md)の実Kata環境での製品経由の一連の起動・実行・取消・回収は、現時点の自動受入試験には含まれていません。上記の単体テストとCLI経由の実Kata受入結果を区別してください。

## 実Kataで確認する

[Kataの配置](kata.md)を終えた専用Linuxノードで実行します。
試験はVM・イメージ・ネットワークを作成します。既存作業と分けて実行してください。

```bash
make kata-e2e
```

このターゲットはCLIとhelperをビルドし、`CODEX_DOCK_KATA_E2E=1`・`CODEX_DOCK_KATA_POLICY_E2E=1` と対象CLIのパスを指定して、`TestKataAcceptance`・`TestKataWorkspaceSharing`・`TestKataPolicyFeatures` を実行します。
通常利用者として実行してください。共有試験は別HOMEと架空のAPIキーを使い、実認証ファイルをマウントしません。
前提条件を満たさない場合は、skipではなく失敗として扱います。
通常の `go test ./...` はこの実機試験を有効にしません。

主に、別kernel、guest権限・資源制限、ホストVM資源制限、Git、readonly/追加mount、PTY、通信遮断、停止・同一IDでの再起動・削除・孤児回収を確認します。
カスタムDockerfileのビルドとOCI import、管理サービス再起動中のVM維持・連続ログ、ローカルResponses fixtureを使うCodexのツール実行も対象です。

[共有試験](workspace.md#検証する)では、CLIのカレントディレクトリ、双方向のファイル操作、開いたままのファイル、UID/GID、権限、読み取り専用、追加マウント、worktree、再起動も確認します。
ホスト由来のinotify通知とNodeのポーリング監視も区別して確認します。

別所有者のディレクトリとsetgidも確認する場合は、専用fixtureを追加します。
親は通常利用者だけが開ける一時ディレクトリとし、試験対象だけに別UID/GIDを設定します。

```bash
owner_fixture=$(mktemp -d)
sudo install -d -o 12001 -g 12002 -m 0777 "$owner_fixture/project"
sudo install -o 12001 -g 12002 -m 0600 /dev/null "$owner_fixture/project/foreign-private"
sudo install -d -o 12001 -g 12002 -m 2777 "$owner_fixture/project/setgid"
CODEX_DOCK_KATA_OWNER_FIXTURE="$owner_fixture/project" make kata-e2e
```

終わったら、その一時ディレクトリだけを削除してください。
fixtureを指定しない場合も、明示UIDの試験は実行します。`--user dir` の対象は呼出元所有の一時ディレクトリになります。

## 実AIとの通信を確認する

実APIとの通信は、認証情報と外部APIの利用権限が必要な別試験です。
認証情報をDockerfileやイメージに含めず、ホストから認証プロキシにだけ渡します。
結果を確認するときは、単なるHTTP 200と、完了したAI応答を区別してください。

| 通信方式 | 成功条件 |
|---|---|
| WebSocket | トンネル確立と、期待した最終応答 |
| HTTP SSE | 明示的なHTTP POST、応答イベント、完了イベント、期待した最終応答 |
| 不正認証 | プロキシ側の401と、上流へ接続しないこと |

Codexの設定でWebSocketを無効にしたつもりでも、使用版によってはWebSocket経路が使われます。
SSEの確認では、実際のHTTP経路とイベントを照合します。

## 確認済みの範囲

2026-10-17時点の記録です。試験環境はcontainerd 2.4.2、Kata 4.2.0 runtime-rs、Dragonball、BuildKit 0.34.0です。

| 試験 | 結果 |
|---|---|
| 実Kata受入 | 全項目PASS。共有設定変更後も再実行し、ワーカーの機能・隔離・ライフサイクルを確認 |
| 共有・所有者の実Kata受入 | 7ケースPASS。既定1000:1000、明示・dir指定12001:12002、0600拒否、setgid、双方向更新、開いたままのファイル、readonly、worktree、再起動 |
| ホストからの変更監視 | inotify通知は届かない。VM内の通知を対照に確認し、NodeのポーリングはPASS |
| 共有修正の回帰 | 全Goテスト、sandbox / kataguard / cmdのrace、vet、macOS arm64クロスビルド、設定生成テスト成功 |
| 認証修正の回帰 | 全Goテスト、race、vet、macOS arm64クロスビルド成功 |
| 既存auth.jsonによる実推論 | Codex 0.163.0でWebSocketと明示的HTTP SSEの最終応答を確認 |
| 認証情報の分離 | ワーカーにはplaceholderと空のrefresh tokenのみ。元auth.jsonは変更なし |
| 後片付け | 試験VM・ネットワーク・一時ルール・専用状態を回収 |

実機受入の一部は、外側にDockerを使った使い捨て環境で実施しました。
その内部の実行ノードではDocker daemonやDocker CLIを起動していません。
実auth.jsonの試験と今回の共有修正後の両受入試験は、専用のネイティブcontainerdで実施しました。
既存Docker/containerdサービスは変更していません。共有試験には架空の認証情報だけを使いました。

今回の実通信試験には、長期間の実OAuth更新、実Anthropic認証による推論、対話キーボード操作を含めていません。
Anthropicの認証・HTTP/WS転送とOAuth更新はfixtureで確認しています。

旧 `scripts/e2e-docker.sh` は移行前のDocker経路用です。現行構成の受入には `make kata-e2e` を使います。

## ドキュメントを確認する

Pythonの仮想環境で依存を入れ、strictビルドを実行します。

```bash
python3 -m pip install -r requirements-docs.txt
mkdocs build --strict
```

ページ・見出しへのリンク切れ、ナビゲーションへの掲載漏れをエラーとして確認します。
プレビューは `mkdocs serve` で起動できます。

## 新規導入・更新・復旧を検証する

専用のLinux/KVMホストから `scripts/verify-setup.py` を実行します。
外側の使い捨てUbuntuを作るためだけにDockerを使い、内部の製品はネイティブcontainerd/Kata/Dragonballで実行します。
既存の通常CIはroot権限を使わない設定生成・配置・復旧の単体テストだけを実行します。

`deploy/kata/Dockerfile.verify` はsystemdとOS依存パッケージを準備します。
`setup` 本体が固定配布物を展開して、専用サービスを新規導入します。
内部ノードへホストPID、ホストDockerソケット、外部ネットワーク、実認証情報は渡しません。
containerdのデータは、イメージ展開に必要なwhiteoutを扱える専用ボリュームへ配置します。

ソースのルートで、Go・Make・Python・Dockerを使って試験資産を作成します。
イメージのビルドにproxy/CAが必要な環境では、[Kataの通信設定](kata.md#3-ネットワークとサービスを有効にする)と同様に環境を整えてください。

```bash
make node-bundle VERSION=setup-test-a
mv dist/node-bundle dist/node-bundle-a
make node-bundle VERSION=setup-test-b
mv dist/node-bundle dist/node-bundle-b
mkdir -p .kata-verification/setup-e2e
CGO_ENABLED=0 go test -c ./internal/sandbox -o .kata-verification/setup-e2e/acceptance.test
docker build -t codex-dock-setup-node:verification -f deploy/kata/Dockerfile.verify deploy/kata
docker build -t codex-dock:kata-guard docker/sandbox
docker build -t codex-dock-proxy:policy-verification -f docker/proxy/Dockerfile .
docker save -o .kata-verification/setup-e2e/images.tar codex-dock:kata-guard codex-dock-proxy:policy-verification
CODEX_DOCK_DISPOSABLE_SETUP=1 python3 scripts/verify-setup.py \
  --bundle dist/node-bundle-a --next-bundle dist/node-bundle-b \
  --images .kata-verification/setup-e2e/images.tar \
  --acceptance .kata-verification/setup-e2e/acceptance.test
```

必要な場合だけ `--sudo-docker` を付けます。配布物を準備済みなら `--cache PATH --offline` で検証できます。
スクリプトはホストの既存サービスを変更せず、自分が作ったラベル付きノードと専用ボリュームを最後に回収します。
証跡は `.kata-verification/setup-e2e/<識別子>/` に残します。

確認範囲は、計画時にサービスを作らないこと、新規配置、通常利用者の `init`、実Kata受入、稼働資源がある場合の更新拒否、正常更新時のイメージ保持、新サービスの起動失敗からの自動復旧、明示的なロールバックです。
OSパッケージのネットワーク取得、ホスト再起動、外部AI推論はこの試験に含みません。


## 追加制御の検証（2026-10-19）

P0/P1の追加後に、既存サービスとは別のcontainerd・BuildKit・Kataノード・cgroup・状態領域で検証しました。架空の認証情報だけを使用しています。

- `make build`、全パッケージの `make test`（race/coverage）、`make vet`、`make lint`、`mkdocs build --strict` が成功。
- macOS arm64のクロスビルドと、配置・導入スクリプトのPythonテストが成功。
- `make kata-e2e` が成功。既存のVM隔離・ツール実行・ノード再起動・認証・通信・停止・削除と、共有/所有者の既存7ケースを確認。
- 追加の実機試験で、Kataの固定IPから選択済みプロキシと実効policy IDを特定できることを確認。
- `TestKataPolicyFeatures` で、mask・個別readonly・rootfsのread-only mount flag、hook/readiness、symlinkを拒否するファイル送受信、exec環境変数/作業ディレクトリ/取消、PTYリサイズ、背景ジョブ、ノード再起動後のジョブとログの回収を確認。
- Go APIの新しい設定を使わない場合、既存要求IDのfingerprint入力が旧版と一致することをgolden testで確認。

[追加制御の一覧と運用条件](sandbox-features.md)も参照してください。寿命・保持期限の適用には `reap --watch` または定期的な `Reap` 呼出しが必要です。実APIキーを使う外部AI推論と、実SSH/JDKサービスへの接続は今回の受入試験には含めていません。認証brokerの経路制限・失効・redirect、独自CA・多段proxyはローカルfixtureで検証しています。
