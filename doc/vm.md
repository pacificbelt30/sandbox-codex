# Kata VMの管理

`codex-dock vm`は、本プロダクトのKataワーカーを名前またはIDで管理する。既存のroot管理サービスを利用し、containerdの記録、永続的なVM予約台帳、ホストの専用cgroupと実プロセスを照合する。通常CLIへroot権限を追加しない。[Kataの配置手順](kata.md)でCLIと`codex-dock-node`を同じ版へ更新する。

```bash
codex-dock vm ps
codex-dock vm ps -a
codex-dock vm ps --watch 5s
codex-dock vm ps -a --json
codex-dock vm inspect worker-name
codex-dock vm stop worker-name
codex-dock vm start worker-name
codex-dock vm rm worker-name
```

新規VMの作成は既存の`codex-dock run`を使う。`vm ps`は実行中のVMに加え、不明・不整合・孤立資源を表示する。`-a`では停止済み・未起動のcontainerdワーカーも表示する。IDにはコンテナIDと予約IDの両方を使用でき、12文字以上の一意な前方一致も扱う。名前とIDの衝突や曖昧な前方一致は拒否する。`--watch`は指定間隔で照合を繰り返し、結果を順に出力する。常駐の自動削除処理は追加しない。

`inspect`と`--json`には、containerd状態、予約ID、cgroupパス、観測したDragonball shim / IO loggerのPID、実際のCPU・メモリ・PID制限、メモリ使用量、照合上の問題を含む。APIキー、環境変数、タスク本文は含めない。

## 状態の意味

| 状態 | 意味 |
|---|---|
| `running` / `paused` | containerdの状態と、固定資産に一致するホストプロセス・所属cgroupを確認した |
| `exited` / `created`など | containerdに保存されたワーカーが存在し、対応する実行中VMプロセスがない |
| `inconsistent` | containerdがrunningだがVMが見つからない、停止済みなのに実プロセスが残る、IDや設定が一致しないなど |
| `orphan-running` | 対応するcontainerdワーカーがなく、専用cgroupにプロセスが残っている |
| `orphan-empty` | 対応するcontainerdワーカーがなく、空のcgroupまたは予約記録が残っている |
| `unknown` | containerdへ問い合わせできず、台帳と実体を完全に照合できない |

`host_verified`はホストプロセスの実体・コンテナID・cgroup制限の確認結果。guestアプリケーションが応答していることや、ゲストの起動後の権限状態を保証するヘルスチェックではない。起動・再起動時のguest検証は引き続き既存のbootstrapゲートが担当する。複数の台帳を単一のトランザクションで取得することはできないため、containerd側で同時に状態が変わると一時的に不整合として表示する場合がある。

## 停止・再起動・削除

通常ワーカーの`vm stop / start / rm`は既存のManagerを通す。停止時の認証失効、再起動時のゲスト検証と認証復元、削除時のネットワーク・管理worktree回収と再試行記録を維持する。停止したcontainerdワーカーは再起動用に残る。実行中ワーカーの削除は`vm rm --force NAME`で明示する。

containerdオブジェクトが既に消えていても、削除途中の記録があれば`vm rm NAME|ID`はその記録を使って回収を続ける。root予約台帳には作成時刻とプロファイル、検証後にはコンテナIDと名前を保存する。予約台帳・削除記録・専用cgroupはワーカーへmountしない。

## 孤立した資源の回収

```bash
# 空の孤立予約だけを回収する。停止済みcontainerdワーカーは削除しない。
codex-dock vm prune

# 特定の孤立予約を回収する。
codex-dock vm rm ALLOCATION_ID

# 実行中の孤立VMは、全占有プロセスを再検証してから明示的に停止する。
codex-dock vm stop --force ALLOCATION_ID
```

containerdへ問い合わせできない場合は孤立と判定せず、停止・回収を拒否する。予約台帳がない古いgroupや、別プロセスの混入、変更された資産・制限は自動で採用しない。コンテナIDが分かる場合は、操作直前にもその正確なIDがcontainerdに存在しないことを確認する。

コンテナ作成前の空の予約は起動処理と区別するため、作成から5分を経過するまで回収を拒否する。実行中の孤立資源は`--force`だけでは十分ではなく、専用group内の全プロセスが固定したDragonball shim / IO loggerであり、同じworker IDに属することを再確認する。停止にはカーネルの`cgroup.kill`を使い、任意のPIDへのkillや他サービスのプロセス移動は行わない。対応しないカーネルや所有確認できない資源ではエラーを返す。

孤立VMの回収は、そのVMのプロセス・予約・専用cgroupが対象。製品外からcontainerdオブジェクトを消して削除記録も残っていない場合、元のネットワーク・認証プレースホルダー・worktreeの完全な回収情報は復元できないため、別途確認する。単にホスト上の全VMMプロセスを走査して管理対象に取り込む機能ではない。他製品のKata VM、runcコンテナ、一般のQEMU VMは管理対象外。

## 動作確認

一覧の実PID・資源、管理サービス再起動時のVM維持、停止・同じIDでの再起動・認証復元・削除、孤児照合と回収を実機で確認しています。
詳細は[テストと動作確認](testing.md)を参照してください。

JSONの `engine_state` と `engine_available` は管理層の状態です。
PIDには組み込みDragonballの `runtime` と、shimが所有する `logger` を表示します。
`host_verified` 単独ではguestアプリケーションの応答は確認できません。
