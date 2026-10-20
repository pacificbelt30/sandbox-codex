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

[Go API](go-api.md)の実機受入 `TestKataGoAPI` は `make kata-e2e` に含まれます。ケース別の架空資格情報でOpen → Launch → Exec → Stop → RemoveRequest、small/standardの別boot ID・ゲスト制限・ホスト予約・削除後の割当回収、取消済みLaunch context、Execの入力EOF・実行中取消とプロセス回収、要求IDの再送・競合・削除後の再送、再接続後の照合、VMへの資格情報漏れ、ケース間のトークン拒否、実行IDの失効を確認します。

追加の障害系受入 `TestKataAPIIntegration` には別ターゲット `make kata-api-e2e` を用意しています。実行中のLaunch取消、OAuth更新・保存失敗・版競合・proxy再起動、通信中ストリームの失効などの対象は[公開Go APIの障害系受入](#公開go-apiの障害系受入)を参照してください。応答を通信路で破棄する故障注入とGo API経由のワーカー別通信許可は、両ターゲットともまだ含みません。単体テストと実Kata受入結果を区別し、成功記録がある版・範囲を確認してください。

## 実Kataで確認する

[Kataの配置](kata.md)を終えた専用Linuxノードで実行します。
試験はVM・イメージ・ネットワークを作成します。既存作業と分けて実行してください。

```bash
make kata-e2e
```

このターゲットはCLIとhelperをビルドし、`CODEX_DOCK_KATA_E2E=1`・`CODEX_DOCK_KATA_POLICY_E2E=1` と対象CLIのパスを指定して、`TestKataAcceptance`・`TestKataWorkspaceSharing`・`TestKataPolicyFeatures`・`TestKataGoAPI` を実行します。
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

## 2026-10-19の実KVM追試

専用の使い捨てUbuntu 26.04ノードで、Kata 4.2.0 / Dragonball、containerd 2.4.2、BuildKit 0.34.0、runc 1.4.3を実KVM上で実行しました。外側のDockerはノードの隔離用で、内部は製品専用containerdです。実API資格情報は使わず、ケース別の架空資格情報を使いました。

| 試験 | 結果 |
|---|---|
| 既存実Kata受入 | PASS。別boot ID、Codexの実ツール実行、Git・PTY・通信分離・ホスト/管理ポート拒否、停止・再起動・削除・孤児回収 |
| 共有・所有者 | 7ケースPASS。readonly/追加mount、明示UID/GID、setgid、worktree、双方向変更、再起動、Nodeのポーリング |
| Go API | PASS。Open → Launch → Exec → Stop → RemoveRequest、再送・再接続照合・競合拒否、削除再試行、入力EOF、実行中Exec取消とプロセス回収 |
| ケース別認証・資源 | PASS。ゲストの既知の認証保存先・環境に元資格情報なし、ケース間トークン拒否、実行ID失効、small/standardのゲスト制限・ホスト予約・割当回収 |
| 配置・更新・復旧 | PASS。新規配置、稼働中更新拒否、正常更新時のイメージ/ファイル保持、新サービス起動失敗からの自動復旧、明示rollback、残存資源なし |
| コード検査 | 全Goテスト、vet、影響範囲のrace、配置Pythonテスト14件成功 |

実機で検出した要求ID照合のcontainerd namespace不足、入力EOFによるExec取消、削除済みworkerのNotFound分類欠落を修正しました。proxy Dockerfileのprotocol/buildラベルも現行APIと一致させました。

初回にはCodex fixtureの45秒タイムアウトとSSE単体試験の一度の失敗があり、再試験は成功しました。SSEは単独5回でも成功しています。最終結果と初回の失敗を区別してください。

この追試は冒頭のGo API受入範囲です。実行中Launch取消・通信路での応答破棄、実Kata経由のOAuth更新/保存失敗/版競合/認証proxy再起動、通信中ストリームの失効、Go APIのワーカー別ドメイン許可を含む全要件の合格判定はまだ完了していません。

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
CGO_ENABLED=0 go test -c -ldflags "-X github.com/pacificbelt30/codex-dock/internal/buildinfo.Version=setup-test-a" ./internal/sandbox -o .kata-verification/setup-e2e/acceptance.test
docker build -t codex-dock-setup-node:verification -f deploy/kata/Dockerfile.verify deploy/kata
docker build -t codex-dock:kata-guard docker/sandbox
docker build --build-arg CODEX_DOCK_BUILD_VERSION=setup-test-a -t codex-dock-proxy:policy-verification -f docker/proxy/Dockerfile .
docker save -o .kata-verification/setup-e2e/images.tar codex-dock:kata-guard codex-dock-proxy:policy-verification
CODEX_DOCK_DISPOSABLE_SETUP=1 python3 scripts/verify-setup.py \
  --bundle dist/node-bundle-a --next-bundle dist/node-bundle-b \
  --images .kata-verification/setup-e2e/images.tar \
  --acceptance .kata-verification/setup-e2e/acceptance.test
```

Go APIだけを再試験する場合は `--acceptance-run '^TestKataGoAPI$'` を指定できます。既定では3つの受入試験を実行します。

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


## 公開Go APIの障害系受入

対応するCLI/node、ワーカーイメージ、ベースproxyイメージ、standalone BuildKitを配置した専用Linux/KVMホストで `make kata-api-e2e` を実行します。`CODEX_DOCK_KATA_NODE_SOCKET` と `BUILDKIT_HOST` で対象を明示できます。`CODEX_DOCK_KATA_TEST_IMAGE` と `CODEX_DOCK_KATA_PROXY_IMAGE` は利用するベースイメージを選びます。ビルド版はnodeと一致させてください（例：`make kata-api-e2e VERSION=dev`）。

スクリプトは `internal/authproxy` のテスト実行ファイルを入れた一意のfixtureイメージを作ります。本物のproxy handler・Unix credential adapterを通し、上流だけをプロキシ内の架空HTTPサーバーへ差し替えます。実APIキーや外部AI推論は使いません。OAuth更新の実Kata fixtureはOpenAI型で、実Anthropic接続は含みません。fixtureイメージを運用に使わないでください。ワーカーとケースproxyを最後に削除し、スクリプト所有のfixtureイメージもテストが回収します。BuildKit cacheとホスト共通egress networkは通常のビルド・ノード管理に従います。

`TestKataAPIIntegration` の対象は、Createの再送とLaunchとの衝突、起動前にhookが動かないこと、Start後のreadiness、small/compactのホストcurrent/peak/OOMとcompact 2台の同時稼働、実要求のモデル拒否、実行中ジョブ/streamのidle回収除外、RevokeRunによる通信中streamの終了と再利用拒否、hook中のLaunch取消と要求台帳・資源回収、OAuth保存失敗・版競合での失効、正常更新とproxy再起動での保存世代再取得、古いtoken拒否と停止/再起動での再有効化です。


## 製品組み込み改善の検証（2026-10-20）

既存サービスから分離したcontainerd・BuildKit・Kataノード・cgroupで、`make kata-e2e VERSION=dev`（当時の対象はAcceptance・WorkspaceSharing・PolicyFeatures、400.698秒）と `make kata-api-e2e VERSION=dev`（受入本体228.339秒）が成功しました。後者はcompact・lifecycle・保存失敗・版競合・proxy再起動の5ケースすべてが成功し、参照中イメージの削除拒否と、最後の試験用イメージ削除・参照消失まで確認しています。取消後のinventoryで、余分なVM・ホスト割当・実プロセスが残らないことも確認しました。

| compactの観測 | ホストcurrent | ホストpeak | 条件 |
|---|---:|---:|---|
| 起動後の待機 | 144850944 B（約138 MiB） | 144850944 B | guestワーカー上限512 MiB、ホスト上限768 MiB |
| 256 MiB確保ジョブ実行中 | 424894464 B（約405 MiB） | 425246720 B（約406 MiB） | 別のcompact VMも同時稼働 |
| 同時稼働の2台目 | 156971008 B（約150 MiB） | 157384704 B | 待機中 |

両VMでguestの `memory.max=536870912` とguest/hostのOOMカウンター0を確認し、負荷ジョブが意図的な停止まで動き続けることも確認しました。同一ホスト・架空HTTP上流での限定した負荷試験です。一般的な依存導入・大規模ビルド・実AI CLI負荷の必要量を保証しません。standard/smallの既存上限と既定値は維持しています。

`make build`、全パッケージrace/coverageテスト、vet、lint、macOS arm64クロスビルド、ドキュメントstrictビルドも成功しました。公開APIの要求台帳照合に必要なcontainerd namespaceを補い、OAuth保存先の一般エラーと空の保存版でもケースと通信中leaseを失効させることを試験しています。運用条件とモデル制限時の対応経路は[追加制御](sandbox-features.md)を参照してください。実認証情報・外部AI推論・systemd user unitの常駐稼働はこの受入には含めず、既存サービスへの更新適用もしていません。

検証終了後、専用ワーカー・proxy・ネットワークが0件であることを確認し、検証用containerd・BuildKit・ノード・cgroup・socket・状態領域を回収しました。既存containerdとノードの稼働は継続しています。


### リモート統合後の追試

上記の改善とGo API実機受入の追加を統合した版で、`make kata-e2e VERSION=dev` のAcceptance（184.97秒）・WorkspaceSharing（8ケース、141.79秒）・PolicyFeatures（46.36秒）が成功しました。GoAPIは初回に試験用StateDirの権限チェックで失敗したため、保存先を既存の0700 fixtureディレクトリ配下へ変更し、`TestKataGoAPI` だけを再実行して成功しました（70.319秒）。製品の権限チェックは維持しています。

統合版の `make kata-api-e2e VERSION=dev` も5ケースすべて成功しました（223.575秒）。全Goパッケージのrace/coverage・vet・lint、配置Pythonテスト14件、補助スクリプトテスト2件、macOS arm64クロスビルドも成功しています。試験用コンテナ・タスク・ネットワークが0件であることを確認してから、専用containerd・BuildKit・ノードと状態領域を回収しました。
