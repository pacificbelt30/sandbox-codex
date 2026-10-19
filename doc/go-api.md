# 製品組み込み用 Go API

`github.com/pacificbelt30/codex-dock/pkg/cxd` は、同じホストの製品からワーカーを操作する Go パッケージです。新しいサーバーを公開しません。内部では既存の `cxd node serve` が提供する root 管理の Unix ソケットを使用します。呼び出し元にはそのソケットへの権限が必要です。この権限では任意のイメージ、コマンド、ホストマウントを指定できるため、信頼したホストプロセスだけに付与してください。

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

`Open` はソケットとノードのバージョンを確認します。`Launch` には、ビルド済みのイメージと起動中の認証プロキシが必要です。CLI の対話的なプロキシ起動、イメージの自動ビルド、`codex-dock.toml` の読み込みは行いません。製品側が実行条件を決めて `LaunchRequest` に渡します。実 API キーや OAuth トークンを `LaunchRequest` や VM に渡さないでください。プロキシの管理シークレットはホスト側の `Options` のみで扱います。

## 再送と復旧

`RequestID` はノード全体で一意にし、組織・プロジェクト・実行 ID・試行番号・接続バージョンなど、製品が区別したい情報を含めてください。同じ ID を別のオプションで再利用すると `ErrConflict` です。ID と設定内容はハッシュ化してノードの永続台帳に記録されます。元の ID、タスク本文、プロキシ管理シークレットは記録しません。

同じ ID を同時に送ると、一方だけが起動を進めます。もう一方には `ErrInProgress` が返ります。起動応答が失われても、同じ ID の再送で既存ワーカーを照合します。起動中のプロセスが消えた場合、台帳と containerd・予約台帳を照合してから再実行を許可します。状態が確定できない間は新たな VM を作らず、`ErrInProgress` を返します。完了済みワーカーを削除した後に同じ ID を再送すると `ErrRequestGone` になり、再作成しません。新しい試行には新しい ID を使ってください。

`Launch` がエラーとともに `Worker.ID` を返した場合、VM または削除記録が残っている可能性があります。`Get` と `Remove` で照合・回収してください。`ErrOutcomeUnknown` は起動後の確認応答が失われたことを示し、同じ ID で再送できます。`ErrCleanupPending` は `Remove` の再試行が必要です。

`context.Context` の取消は `Exec` 中のゲストプロセスを止めます。`Wait` の取消は待機のみを止めます。`Launch` は取消後も回収を試みます。VM を止めて認証トークンを失効させるには `Stop`、関連資源を削除するには `Remove` を呼んでください。

## エラーと互換性

`errors.Is` で `ErrNotFound`、`ErrConflict`、`ErrInProgress`、`ErrCleanupPending`、`ErrRequestGone`、`ErrCapacity`、`ErrVersionMismatch`、`ErrOutcomeUnknown` を判定できます。`context.Canceled` と `context.DeadlineExceeded` も保持します。元のエラーは `errors.Unwrap` で参照できます。

公開 Go パッケージは node の内部 DTO をシグネチャに出しません。現時点では CLI・ライブラリ・ノードを同じビルドで配置してください。版が違う場合、`Open` は `ErrVersionMismatch` を返します。ノードの Unix ソケット上の HTTP/JSON は内部プロトコルであり、外部向けの互換性を約束しません。
