# 製品組み込み用 Go API

**日本語** | [English](en/go-api.md)

`github.com/pacificbelt30/codex-dock/pkg/cxd` は、同じホストの製品からワーカーを操作する Go パッケージです。新しいサーバーを公開しません。内部では既存の `cxd node serve` が提供する root 管理の Unix ソケットを使用します。呼び出し元にはそのソケットへの権限が必要です。この権限では任意のイメージ、コマンド、ホストマウントを指定できるため、信頼したホストプロセスだけに付与してください。

## 利用前の準備

1. [実行ホスト](setup.md)に node、containerd、Kata、BuildKit を配置し、製品プロセスから node の Unix ソケットへ接続できるようにする。既定のソケットは `/run/codex-dock-node.sock`。別の場所では `Options.NodeSocket` を指定する。
2. [認証プロキシ](getting-started.md#3-プロキシとイメージを用意する)を起動し、製品から使うイメージをビルドまたは import する。`Open` はプロキシやイメージを起動・作成しない。
3. 製品と node を対応する版で配置する。`Open` は node の接続・検証と Go クライアント用プロトコルの確認を行い、不一致を `ErrVersionMismatch` で返す。CLI と node を更新するときは、製品に組み込んだ Go パッケージも同じソース版へ更新する。

プロキシの既定接続先は、ホスト側管理 API が `http://127.0.0.1:18081`、ワーカー側認証 API が `http://codex-auth-proxy:18080`、一般通信プロキシが `http://codex-http-proxy:18082` です。配置が異なる場合は `Options.ProxyAdminURL`、`ProxyContainerURL`、`HTTPProxyURL` を指定します。管理 API にシークレットを設定している場合は `Options.ProxyAdminSecret` にホスト側だけで渡してください。

## 実行例

```go
package main

import (
    "context"
    "errors"
    "os"
    "time"

    "github.com/pacificbelt30/codex-dock/pkg/cxd"
)

func main() {
    ctx := context.Background()
    client, err := cxd.Open(ctx, cxd.Options{
        ProxyAdminSecret: os.Getenv("CODEX_DOCK_PROXY_ADMIN_SECRET"),
    })
    if err != nil { panic(err) }
    defer client.Close()

    worker, err := client.Launch(ctx, cxd.LaunchRequest{
        RequestID:  "product/project-7/job-123/attempt-1",
        ProjectDir: "/srv/work/project-7",
        Image:      "codex-dock:latest",
        Profile:    "small",
        NoGitMetadata: true,
    })
    if errors.Is(err, cxd.ErrInProgress) {
        // 同じ RequestID で後から再試行する。
        return
    }
    if err != nil { panic(err) }
    defer client.Remove(context.Background(), worker.ID, true)

    code, err := client.Exec(ctx, worker.ID, cxd.ExecRequest{
        Argv: []string{"codex", "exec", "Fix the tests"},
        Stdout: os.Stdout,
        Stderr: os.Stderr,
    })
    if err != nil { panic(err) }
    if code != 0 { panic("codex exited unsuccessfully") }
    if err := client.Stop(ctx, worker.ID, 10*time.Second); err != nil { panic(err) }
}
```

`Agent` を指定しない `Launch` は、`Exec` を複数回呼べるよう待機状態のワーカーを起動します。`Agent: "codex"` または `"claude"` を指定すると、その CLI をメインプロセスとして起動します。`Exec` はコマンドの終了コードと通信エラーを別々に返します。`Wait` はメインプロセスの終了待ちです。待機状態のワーカーに対する個々の `Exec` の終了待ちではありません。

| 操作 | 契約 |
|---|---|
| `Launch(ctx, request)` | `RequestID` を使ってワーカーを作成・起動し、ID を返す。`ProjectDir` は絶対パス。現時点では作成と初回起動を分離しない |
| `Get` / `List` | ワーカーの状態を照会する |
| `Exec` / `Wait` | 既存ワーカーでコマンドを実行する／メインプロセスの終了を待つ |
| `Create(ctx, request)` | 初回起動前のKataワーカーを作成。VM・hook・agent・生きたトークンは開始しない。作成と起動を分ける場合に使用 |
| `ResourceUsage(ctx, worker)` | 検証済みVMのホストcgroupの現在値・peak・OOM kill数と、資源プロファイルの上限を別々に取得 |
| `Start` / `Stop` / `Remove` | 停止済みワーカーの再起動／停止／回収。`Stop` は認証トークンを失効させる |
| `Request` / `RemoveRequest` | 要求IDから永続起動状態を照会する／応答消失後も同じ要求IDで回収を再試行する |
| `Profiles(ctx)` | 稼働中ノードの CPU、guest RAM、ホスト側予約メモリ、PID 上限を取得する。値は予約・制限の契約であり、実測 RSS ではない |
| `Capabilities()` / `ProxyCapabilities(ctx)` | nodeのビルド版・プロトコル・対応機能／接続先認証Proxyの対応機能を調べる |

`LaunchRequest` ではモデル、承認モード、追加マウント、read-only rootfs、Git metadata の非共有なども指定できます。指定しないイメージは `codex-dock:latest`、資源プロファイルは `standard` です。小型の `small` または軽量用途の `compact`（guest 512 MiB／host 768 MiB）を選ぶ前に `client.Profiles(ctx)` で稼働中ノードの値を確認してください。

`Open` はソケットとノードのバージョンを確認します。`Launch` には、ビルド済みのイメージと起動中の認証プロキシが必要です。`client.PrepareProject(ctx, dir, requestID, log)` は CLI と共通の `codex-dock.toml` 検証を使い、必要なイメージを BuildKit で作成して `LaunchRequest` を返します。`client.Build` と `client.BuildTemplate` も個別に利用できます。製品側で設定を確認・補足してから `Launch` を呼べます。

`PrepareProject` はグローバル設定や実認証情報を読みません。プロジェクトの `egress_allow_domains` は `LaunchRequest.EgressAllowDomains` に反映されます。Go API は既存のCLIと同じ識別子の専用egress proxyを、互換イメージから起動・再利用します。`Options.EgressPolicy` にホストの上限ルールを設定でき、ワーカーの許可ドメインはその範囲を広げられません。プロキシイメージは事前にビルドしてください。CLI の対話的なプロキシ起動は行いません。実 API キーや OAuth トークンを `LaunchRequest` や VM に渡さないでください。プロキシの管理シークレットはホスト側の `Options` のみで扱います。

`PrepareProject` が返す設定を製品側で確認し、必要なホスト側の制約を加えてから `Launch` に渡してください。利用者・ケース・実行の認可と業務状態は製品側で管理します。ケース別の資格情報と実行identityを扱う場合は[ケース別認証](credential-store.md)を参照してください。

## 再送と復旧

`RequestID` はノード全体で一意にし、利用者またはケース・プロジェクト・実行 ID・試行番号・接続バージョンなど、製品が区別したい情報を含めてください。同じ ID を別のオプションで再利用すると `ErrConflict` です。ID と設定内容はハッシュ化してノードの永続台帳に記録されます。元の ID、タスク本文、プロキシ管理シークレットは記録しません。

`UseWorktree: true` でブランチを省略すると、要求 ID から専用ブランチ名を作ります。起動に失敗して worktree が回収された後も、同じ要求の再送ならそのブランチを再利用できます。

同じ ID を同時に送ると、一方だけが起動を進めます。もう一方には `ErrInProgress` が返ります。起動応答が失われても、同じ ID の再送で既存ワーカーを照合します。起動中のプロセスが消えた場合、台帳と containerd・予約台帳を照合してから再実行を許可します。状態が確定できない間は新たな VM を作らず、`ErrInProgress` を返します。完了済みワーカーを削除した後に同じ ID を再送すると `ErrRequestGone` になり、再作成しません。新しい試行には新しい ID を使ってください。

起動leaseが有効な間（5分）は、ワーカーが作成済みでも起動完了とは扱いません。起動側が完了確認を送る前に消えた場合、lease失効まで `ErrInProgress` を返すことがあります。完了確認が台帳に保存済みなら、再送でその結果を返します。

`Launch` がエラーとともに `Worker.ID` を返した場合、VM または削除記録が残っている可能性があります。`Get` と `Remove` で照合・回収してください。`ErrOutcomeUnknown` は起動後の確認応答が失われたことを示し、同じ ID で再送できます。`ErrCleanupPending` は `Remove` の再試行が必要です。

`Request(ctx, requestID)` は `pending`、`complete`、`failed`、`cleanup-pending` の台帳状態を返し、起動中のleaseは公開しません。`RemoveRequest(ctx, requestID, true)` は既存ワーカーと削除記録の回収を試み、前回の回収応答だけが失われた場合も成功として扱えます。製品再起動後も同じ `Options.StateDir` を使ってください。

`context.Context` の取消は `Exec` 中のゲストプロセスを止めます。`Wait` の取消は待機のみを止めます。`Launch` は取消後も回収を試みます。VM を止めて認証トークンを失効させるには `Stop`、関連資源を削除するには `Remove` を呼んでください。

## エラーと互換性

`errors.Is` で `ErrNotFound`、`ErrConflict`、`ErrInProgress`、`ErrCleanupPending`、`ErrRequestGone`、`ErrCapacity`、`ErrVersionMismatch`、`ErrOutcomeUnknown` を判定できます。`context.Canceled` と `context.DeadlineExceeded` も保持します。元のエラーは `errors.Unwrap` で参照できます。

公開 Go パッケージは node の内部 DTO をシグネチャに出しません。現時点では CLI・ライブラリ・node・proxyイメージを対応するビルドで配置してください。Go API 用nodeプロトコルはv2です。版またはプロトコルが違う場合、`Open` は `ErrVersionMismatch` を返します。`Capabilities()` は検証済みnode、`ProxyCapabilities(ctx)` は現在の認証Proxyを報告します。nodeの Unix ソケット上の HTTP/JSON は内部プロトコルであり、外部向けの互換性を約束しません。

[追加のポリシー・実行・回収API](sandbox-features.md)

`Create` の後に `Start` を呼ぶ段階的な起動も可能です。[運用条件](sandbox-features.md#モデル認可と段階的な起動)を確認してください。`Launch` の作成と初回起動が一体である従来の動作は維持します。

### 実行ごとのモデル認可

`LaunchRequest.ModelPolicy` に `&identity.ModelPolicy{AllowedModels: []string{"model-id"}}` を指定すると、実際の上流リクエストのモデルをトークンごとに制限します。Proxy 全体の許可集合との積集合を使います。`nil` は従来どおり Proxy 設定を継承し、空のポリシーは全モデルを拒否します。`Model` はモデル選択にすぎず、認可ではありません。

`ProxyCapabilities.TokenModelPolicy` で対応を確認できます。未対応 Proxy への指定は失敗します。ポリシーは不変の作成条件に含まれ、停止・再起動時も復元されます。モデル制限のあるトークンは HTTP/SSE の検証可能な AI ルートだけを許可し、WebSocket とリダイレクトは許可しません。一般 egress は別ポリシーなので、迂回を防ぐ必要がある実行では `NoInternet` または適切な宛先制限も設定してください。

ワーカーに保存するモデルポリシーは、ラベル名を含むJSON表現で4KiB以内にしてください。

### 接続情報とCodexのHTTP/SSE

`Connections(ctx, workerID)` はゲスト内から使うURL、認証ヘッダー・方式、トークン用環境変数名、トランスポート、モデル制限の有無を返します。トークン値やホストの資格情報は含みません。作成済みでも `Start` 前はトークンが有効ではありません。

`CodexHTTPCommand(ctx, workerID)` はHTTP/SSEを明示する `Argv` と、短命のプレースホルダーだけを含む `Env` を返します。`Argv` に `exec` や `app-server` と引数を追加し、`ExecRequest` に渡してください。生成済みユーザー設定を読まない呼び出し元でも、接続設定はCLIの `-c` で渡されます。`Env` はログに出さないでください。組み込みワーカーのCodex 0.163.0を検証対象にしています。

モデル制限付きの通常起動はWebSocketとリクエスト圧縮を無効化します。制限なしの既定起動は従来どおりです。明示接続ヘルパーも圧縮を無効にし、ProxyがJSONを検証できるようにします。[公式設定リファレンス](https://learn.chatgpt.com/docs/config-file/config-reference)と[CLIによる設定上書き](https://learn.chatgpt.com/docs/config-file/config-advanced)に対応します。
