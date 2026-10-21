# 性能とベンチマーク

**日本語** | [English](en/performance.md)

Kataの資産検証、公開Go APIのExec、認証プロキシの処理を別々に計測します。
[計測結果と実機での回帰確認](testing.md#2026-10-21の性能計測)に、変更前後の値、測定条件、確認済みの範囲を記録しています。

## 高速化の仕組み

資産検証は、ファイルごとの所有者・パス・実行権限・SHA-256の確認を、1回の検証につき最大 `min(4, GOMAXPROCS)` 並列で行います。
毎回すべての資産を読み、設定との結び付けも確認します。[セキュリティ境界](security.md)の検査を維持したまま、空いているCPUを利用します。
総ハッシュ量は変わらないため、CPU飽和時のthroughputや、ディスク読み出しが支配的な環境での改善率は別途計測が必要です。

認証プロキシは、トークン値から発行記録を直接検索します。
ワーカー数に比例する全件走査を避け、発行・復元・失効と同じロック内で索引を更新します。
期限・プロバイダー・モデル制限、既存streamの取消は[認証プロキシ](auth-proxy.md)の説明どおりに適用します。

これらの高速化は実装側で有効になります。専用の有効化フラグはありません。
導入は[CLI・管理サービス・プロキシの更新手順](operations.md#cliプロキシを更新する)に従い、Go APIを使う製品もnodeと同じソース版へ更新します。

## 計測対象を選ぶ

| Benchmark | 計測に含むもの | 計測外のもの |
|---|---|---|
| `BenchmarkWorkerRequest` | 認証ヘッダー解析、発行記録の検索、期限・経路の確認、request contextの生成・解放 | ネットワーク、上流APIへの通信 |
| `BenchmarkLoadPinnedManifest` | 実資産の読込・checksum・所有権・パス・設定の照合 | VM起動、guest内の処理 |
| `BenchmarkKataExec` | Execごとの資産・ホスト・guestの検証、`/bin/true` の実行と終了確認 | VM作成、clientのOpen、warmup、AI推論 |

Proxyのbenchmarkは1・32・256・4,096件の発行記録で、認証成功と未発行トークンの拒否を測ります。
Execのbenchmarkに表示されるallocation値はクライアント側の値です。

## 再計測する

ソースのルートで、[開発用Go環境](testing.md#コードを確認する)を使って実行します。
次の `CODEX_DOCK_BENCH_*` はbenchmarkだけが読む入力です。

### Proxy認証

Kataや実APIへの接続は不要です。

```bash
go test ./internal/authproxy -run '^$' \
  -bench '^BenchmarkWorkerRequest$' -benchmem -count=5 -benchtime=500ms
```

### 固定資産の検証

Linux上で、実際に固定したmanifestのパスを指定します。ファイルへの書き込みは行いません。
root所有のmanifestには、node運用者の読み取り権限が必要です。信頼済みソースから実行してください。

```bash
CODEX_DOCK_BENCH_MANIFEST=/etc/codex-dock/kata-assets.json \
  go test ./internal/kataguard -run '^$' \
    -bench '^BenchmarkLoadPinnedManifest$' -benchmem -benchtime=3x -count=5
```

### 実KataのExec

準備済みの検証ホストで、通常の手順でアイドル状態の管理Kataワーカーを起動し、その完全なIDを指定します。
benchmarkは既存VMで `/bin/true` を実行します。ワーカーの起動と、測定後の停止・削除は呼出側が行います。
`FULL_WORKER_ID` を実際のIDに置き換え、`-ldflags` の `dev` は実行中nodeのBuildVersionと一致させてください。

```bash
CODEX_DOCK_KATA_NODE_SOCKET=/run/codex-dock-node.sock \
CODEX_DOCK_BENCH_WORKER=FULL_WORKER_ID \
  go test -ldflags '-X github.com/pacificbelt30/codex-dock/internal/buildinfo.Version=dev' \
    ./pkg/cxd -run '^$' -bench '^BenchmarkKataExec$' -benchmem -benchtime=3x -count=5
```

manifestのパスやワーカーIDを省略すると、対応するbenchmarkはskipします。skipを計測成功として扱わないでください。

## 変更前後を比較する

同じGo版・CPU条件・資産・ワーカープロファイルで比較し、計測中は他のビルドや試験を止めます。
nodeとclientには同じ測定用BuildVersionを指定します。各組で旧版・新版の順序を入れ替え、複数標本の中央値を比較してください。
page cacheが温まった状態と、初回のディスク読み出しを含む状態も区別します。

性能計測後には[回帰確認](testing.md)を行います。runtime変更は `make kata-e2e`、認証変更は失効・期限切れ・置換・restore・Proxy停止による接続取消も確認します。
試験用VM・コンテナ・ネットワークを回収してから、検証用サービスや管理記録を削除します。
