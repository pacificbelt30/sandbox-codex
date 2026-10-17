# テストと動作確認

テストを「コードの確認」「実Kataでの確認」「実AIとの通信」に分けて説明します。
自動テストの成功だけで、外部APIや任意のカスタムイメージの動作を保証するものではありません。

## コードを確認する

Go 1.24.7以上を使います。race検査にはCコンパイラーも必要です。

```bash
go test ./...
make test
make vet
gofmt -l cmd internal
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

## 実Kataで確認する

[Kataの配置](kata.md)を終えた専用Linuxノードで実行します。
試験はVM・イメージ・ネットワークを作成します。既存作業と分けて実行してください。

```bash
make kata-e2e
```

このターゲットはCLIとhelperをビルドし、`CODEX_DOCK_KATA_E2E=1` と対象CLIのパスを指定して、`TestKataAcceptance` と `TestKataWorkspaceSharing` を実行します。
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
