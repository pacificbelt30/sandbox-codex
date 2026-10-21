# セキュリティ設計

既定はKata runtime-rs / DragonballのVM境界。containerdとroot管理サービスが実行・資源・networkを管理する。[配置と実機検証](kata.md)、[ネットワーク](network.md)、[プロキシポリシー](proxy-policy.md)を参照。

## 資格情報

OpenAI / AnthropicのAPIキー、API用access token、OAuth refresh tokenはauth proxyが保持する。workerには短命のplaceholderだけを渡し、固定API経路を中継する際に実資格情報へ差し替える。Codex loginの互換性のため、ID tokenなどのaccount metadataは返す場合がある。egress proxyには実資格情報をmount・注入しない。

認証プロキシは呼出元のUID/GIDと専用HOMEで、所有者の0600認証ファイルを読み取り専用で開く。capability全削除を維持し、元ファイルの権限は変更しない。

AI APIのHTTP・WebSocketは、発行済みで有効な短命トークンを上流接続前に検証する。未発行・失効・期限切れ・重複・矛盾する認証は401で拒否する。失効・期限切れ・置換時は既存のHTTP streamとWebSocketも終了する。

admin listenerはauth proxyのprimary egress IPにbindし、hostでは127.0.0.1へ公開する。workerから見えるdata listenerにはadmin操作を公開しない。placeholderの失効・再起動時の復元はhostのManagerだけが行う。

## ワーカーの制限と起動ゲート

| 項目 | 条件 |
|---|---|
| kernel | workerごとに別のDragonball VM |
| UID/GID | 解決済みの非root ID。既定はhostのcurrent user |
| capabilities | bounding / effective / permitted / inheritable / ambientをすべて0 |
| NNP・seccomp | `NoNewPrivs=1`、guest `Seccomp=2` |
| guest資源 | CPU 1、memory 2 GiB、PID 512 |
| host VM全体 | CPU 1、memory 3 GiB、swap 0、PID 512、OOM group |
| network | 専用bridge・namespace、静的proxy hosts、直接外向き・host・peer通信を遮断 |
| management | containerd/node/BuildKit socket・資産・台帳・host kernel操作用パスをguestへ渡さない |

固定bootstrapをPID 1として起動し、元のimage entrypointは待機させる。hostのshim/logger実体・worker ID・cgroup所属・上限、guestの別boot IDと実権限・制限を検証したあとで、placeholderを有効にしてnonceで解放する。Kataや資源検証に失敗した場合はエラーになり、runc/QEMUへ自動fallbackしない。

Kata外側の隔離を検証したworkerでは、Codexの内側sandboxを `danger-full-access` に設定する。VMの中でさらにnamespaceを作ることによるseccompとの不整合を防ぐためであり、CLIの承認設定は独立して維持する。Codexの非対話execはCLI自身がapprovalを選ぶ場合がある。

## ホスト側の信頼境界

nodeのoperator groupとBuildKit利用者はroot相当で、image・command・host bind mountを選べる信頼済み利用者である。guestにはoperator socketを公開しない。プロキシはrunc、workerは既定でKata。`--runtime runc` は同じcontainerd上で明示選択する互換モードで、独立kernelを提供しない。

manifestでroot所有・一般ユーザーから書き換え不可のruntime、kernel、guest image、bootstrap、IO logger、containerd設定と本体、network操作ツールを固定する。実プロセスはinode、cmdlineのworker ID、start ticks、実cgroup所属で照合する。manifestはchecksum記録であり、配布物の署名・出所の確認の代わりではない。

資産検証は毎回全ファイルを再読込し、SHA-256を計算する。ファイル間の並列数は1回の検証につきGOMAXPROCS以下・最大4に制限し、更新時刻などを根拠とする検証結果のキャッシュは使わない。[性能計測の手順](performance.md)も参照。

Dragonball終了時のcgroup回収は同じ上限内のsibling leafへ移す。起動失敗の回収と孤児の強制停止は、専用予約の全占有プロセスを確認した場合だけ `cgroup.kill` を使う。他サービスのgroup・任意PID・不明なnamespaceを採用しない。管理層に接続できない場合はunknownとし、孤児として削除しない。

## workspace・Git・削除

workspaceと追加mountのreadonly指定を保つ。worktreeでのcommit互換性を維持するため、共通Git metadataは同じ絶対パスで共有し、必要な場合は書き込み可能にする。VM隔離は、明示的に共有したファイルやGit設定を不変にする仕組みではない。元の `.git` pointerは書き換えない。

削除journalは実オブジェクト削除前に永続化する。network、token失効、cgroup、worktreeの回収が不確実な場合はintentを残し、worktreeを先に消さない。rootのVM予約・network情報・OCI spec・ログはprivateな状態ディレクトリへ保存する。VM inventoryにはAPIキー・環境変数・タスク本文を出さない。

## 制限と確認範囲

ワーカーとプロキシ間は専用ネットワーク上の平文HTTPであり、TLSは提供しない。管理APIやデータポートを公開ネットワークへ直接公開しない。
短命トークンはBearer認証で、送信元コンテナIDとの照合は行わない。盗まれた有効トークンは失効まで利用され得る。
一般通信のルールは選択したegressプロキシ単位で固定する。ワーカー別の許可リストはポリシー別proxyで強制し、同じ規則のワーカーはproxyを共有できる。既定拒否は明示選択であり、省略時の空リストは公開宛て通信を許可する。ホットリロードは提供しない。

[実機試験と実AI試験](testing.md)では、隔離・権限・資源・機能と、既存auth.jsonによるWebSocket/HTTP SSE推論を確認した。
長期間の実OAuth更新、対話キーボード操作、任意のカスタムイメージは別途確認が必要。
隔離の強さはhost kernel・KVM・Kata・guest kernelの更新状態にも依存する。

モデルIDの制限は、ホストが認証プロキシへ明示設定した場合だけ有効です。起動CLIのモデル指定だけでは認可を強制しません。対応経路・WebSocket拒否・別通信経路の扱いは[追加制御](sandbox-features.md#モデル認可と段階的な起動)を参照してください。

Claude CodeはAPIキー・サブスクで仮トークン用の環境変数を分け、実access/refresh tokenをワーカーへ保存しません。実Anthropic推論とサブスク利用枠の検証はfixture試験に含みません。[Claude Code](claude-code.md)と[テスト記録](testing.md#claude-codeの受入)を参照してください。
