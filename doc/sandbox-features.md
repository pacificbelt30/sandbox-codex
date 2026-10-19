# 追加のサンドボックス制御

以下は明示的に選ぶ機能です。未指定時の通信許可・ワーカー保持・作業ディレクトリの既定値は変更しません。新しいノード、プロキシ、ワーカーイメージをそろえてから有効にしてください。古いノードが制限を黙って無視しないよう、ファイル制限・image digest・idle管理・ジョブには能力確認があります。


| 優先度 | 対応項目 | 実装入口 |
|---|---|---|
| P0 A1 | 実効ポリシー・説明・事前検証 | `policy show/check/explain`、Go API |
| P0 A2 | 理由コード付き拒否イベント | `policy events`、TUIのE、Go API |
| P0 A3 | 明示的なプリセット | `--policy-preset`、project config |
| P0 A4 | 診断と能力確認 | `doctor`、`capabilities`、`ProxyCapabilities` |
| P1 A5 | 個別のファイル読み書き制限 | `--deny-read` / `--deny-write` |
| P1 A6 | 実行・ファイル・背景ジョブ・PTY | `exec/files/jobs`、Go APIのsession |
| P1 A7 | 寿命・idle・保持期限 | opt-inの設定と `reap --watch` / `Reap` |
| P1 A8 | 環境固定・hook・readiness | `environment lock`、起動オプション |
| P1 A9 | 独立cloneと変更回収 | `--clone-dir`、`artifact`、files API |
| P1 A10 | 上流・パス・ヘッダーを限定した認証broker | `--credential-routes`、Go callback |
| P1 A11 | SSH/JVM・上流proxy・独自CA | compatibilityフラグとproxy設定 |

既定のdeny-all化、既定のファイルmask・clone・自動削除、TLS MITM、実行中ポリシーの変更、VM snapshot/fork、新しいOS backend、リモート多人数サービスは今回の対象に含めません。

## 通信の確認と設定

```bash
cxd policy check --json
cxd policy show --worker WORKER --json
cxd policy explain api.example.com:443 --resolve --json
cxd policy events --worker WORKER --follow --json
cxd policy presets
cxd run --policy-preset api --egress-allow-domain api.example.com -d
cxd run --policy-preset dependencies -d
cxd run --policy-preset offline -d
cxd proxy configure
```

`policy show --worker` はワーカーが選んだプロキシの実設定とIDを読みます。`--worker` がない場合は、新規起動用のホスト設定です。`explain` は接続を開きません。`--resolve` なしの許可は名前・ポートだけの暫定判定です。指定時は全DNS応答を検証します。実際の接続時にも再検証します。

`offline` と `test` は一般通信を切り離し、固定AI経路は維持します。brokerを設定した場合、その固定認証経路も一般通信ポリシーとは独立して利用可能です。`dependencies` はnpm/Python/Goの代表的なレジストリ、`api` は明示したドメインだけを選びます。どのプリセットもホストの許可上限・拒否ルール・private IP遮断を緩和しません。

既存TUIでは **P** で設定、**E** で拒否履歴を開きます。`proxy configure` はノード停止中でも使えます。許可・拒否の6項目、上流プロキシURL、CAファイルのパスを編集し、宛先を検査して保存できます。秘密値は表示しません。他の設定を保ち、同時編集は検出します。複数行配列など安全に編集できない形式は保存を拒否するため、テキストエディタで編集してください。

保存は実行中の設定を変更しません。既存ワーカーを退避して対象プロキシを明示的に再作成します。詳細は[通信ポリシー](proxy-policy.md)を参照してください。

拒否イベントには時刻、理由コード、接続元、宛先authority、policy IDを含めます。URLのパス・query・ヘッダー・資格情報は記録しません。一般通信プロキシは複数ワーカーで共有するため、履歴も共有です。接続元IPを実行IDと同一視せず、機密性が必要なケースはプロキシも分けてください。プロセス内履歴は512件、CLI/APIは保持された直近1000行を読みます。プロキシのイベント出力は1プロセス10万件までで、上限時は `event_log_limit` を出してそれ以降のログ出力を止めます。プロキシ再作成後の永続監査ログではありません。

## ファイル・実行・回収

```bash
cxd run --deny-read /workspace/.env --deny-write /workspace/config -d
cxd exec --workdir /workspace --env MODE=test WORKER -- npm test
cxd exec -it WORKER -- bash
cxd files get WORKER src/main.go --local main.go.copy
cxd files put WORKER input.json --local input.json
cxd files list WORKER src
cxd jobs start WORKER -- sh -c 'npm test'
cxd jobs list WORKER
cxd jobs logs WORKER JOB_ID --offset 0
cxd jobs kill WORKER JOB_ID
cxd artifact diff WORKER --output changes.patch
cxd artifact commit WORKER --message 'Save staged changes'
cxd run --clone-dir /absolute/new-copy -d
```

ファイル制限は `/workspace/` 以下の明示したパスにOCI mask/readonlyを適用します。未指定時は追加制限をかけません。対象は起動時に存在する通常ファイル・ディレクトリに限り、symlinkやworkspace外への解決は拒否します。将来作るファイルへの継承ルールではありません。パスの別名やハードリンクに対するinode単位の機密ラベルではありません。複数マウントに同じ内容を露出しないでください。

ファイルAPIはguestのPython 3で実行し、各パス要素のsymlinkを拒否します。通常ファイルだけを最大64 MiBまで読み書きし、アップロードは同じディレクトリで原子的に置換します。ディレクトリ一覧は最大10000件です。既存の実行権限やVM起動検証を迂回しません。

PTYではstdout/stderrが一つになります。ストリーミング実行・セッションは切断やcancelで終了します。背景ジョブは切断後も続き、状態とshim所有のログをノード再起動後に復元します。出力は合計16 MiBまで保持し、以後もguest出力は排出します。ログAPIは1回最大1 MiB、次のoffsetと打切り情報を返します。完了ジョブはワーカー削除まで残り、ノード全体で実行セッションを最大4096件保持します。

独立cloneはコミット済みの内容から作り、Gitオブジェクトをハードリンク共有しません。既存worktree方式も維持します。clone先は新規の絶対パスが必要です。cloneは起動失敗時・ワーカー削除時も回収用に残します。再送時は既存cloneを暗黙採用しません。差分出力はHEADに対する追跡済み変更です。未追跡ファイルはfilesで取り出し、commitは事前にstageした変更だけを記録します。

## 寿命と再現性

```bash
cxd run --max-lifetime 2h --idle-timeout 30m --retention 24h -d
cxd reap --dry-run
cxd reap --watch
cxd environment lock IMAGE --source Dockerfile --source package-lock.json
cxd run --image IMAGE --environment-lock codex-dock.lock.json -d
cxd run --start-hook 'npm ci' --readiness 'test -f node_modules/.package-lock.json' --readiness-timeout 2m -d
```

寿命・idle・保持期限は `reap --watch` またはGo APIの定期的な `Reap` 呼出しが適用します。CLI起動だけで監視サービスを常駐配置することはありません。監視が止まれば期限の適用も止まります。idleは最後の管理API実行の時刻で、guestのCPU使用やファイル更新を計測した値ではありません。長いジョブ・対話作業にidle制限を使うと途中停止し得るため、用途に合わせて選んでください。停止はtoken失効を伴い、削除は既存の所有権確認・削除journalを通します。保持期限ゼロは削除しません。

lockはノード上のOCI image digestと指定ファイルのSHA-256を固定し、新規ファイルにだけ書きます。タグまたはsourceが変われば起動を拒否します。依存パッケージ自体のlockfileもsourceに含めてください。起動hookはguest内で認証設定後・agent開始前に順次実行します。probe成功後に起動完了とし、失敗や期限超過では起動を回収します。既定のhook/probe期限は60秒、起動全体には既存の180秒上限もあります。対応ラベルのない旧イメージではhookを拒否します。

プロジェクトの `codex-dock.toml` の `[run]` に `policy_preset`、`deny_read`、`deny_write`、`max_lifetime`、`idle_timeout`、`retention`、argv配列の `start_hooks` と `readiness`、`readiness_timeout` を指定できます。ホストの秘密・追加mount・runtime・プロキシURLは引き続き指定できません。

## 資格情報と互換性

`cxd proxy run --credential-routes /absolute/private-routes.json` は、ホストの0600 JSONを認証プロキシだけにread-onlyで渡します。一般通信プロキシには資格情報を渡しません。例は架空の値です。

```json
[{"Name":"service","Upstream":"https://api.example.com","Paths":["/v1"],"Methods":["GET","POST"],"Header":"X-Service-Key","Prefix":"","Provider":"openai","Secret":"HOST_SECRET"}]
```

guestは `http://AUTH_PROXY/broker/service/v1/...` に、発行済みplaceholderのBearer認証を付けて要求します。固定HTTPS origin・許可パス境界・メソッド・providerを検証してから秘密を注入します。encoded path、WebSocket、上流redirectの追跡は拒否します。秘密値はレスポンスAPIから取得できません。ただし許可した上流が秘密をechoする場合まで防げないため、信頼できるAPIだけを設定してください。設定ファイル変更時は退避・再作成が必要です。組み込みでは `StartCredentialBroker` と `CredentialRoute.Secret` callbackも使えます。独立brokerのlistener・workerネットワークへの配置は呼出し側で明示します。

```bash
cxd run --ssh-proxy --jvm-proxy --ca-bundle /absolute/extra-ca.pem -d
cxd proxy run --upstream-proxy http://proxy.example.com:3128 --ca-bundle /absolute/extra-ca.pem
```

SSHは `GIT_SSH_COMMAND` のCONNECT helperで一般通信ポリシーを通します。SSH鍵・ホストagent socketは自動共有しません。JVMはHTTP(S) proxy propertiesを設定します。CAはワーカーでsystem bundleに追加し、curl/Python/Nodeに適用します。JVMとkeytoolがある場合はsystem truststoreをワーカーのHOMEへコピーしてCAを追加します。プロキシ側のCAはsystem rootsに追加し、HTTPとWebSocket両経路でTLSを検証します。

多段プロキシは検証済みIPへのCONNECTを使います。DNS検証を上流に任せず、private IP遮断を維持します。上流プロキシへのユーザー名・パスワード付きURLは受け付けません。

## Go APIと性能確認

[Go API](go-api.md)には `Policy` / `ExplainPolicy` / `PolicyEvents`、`ExecRequest` の作業ディレクトリ・環境変数・TTY、`StartSession` / `Resize`、ファイル・ジョブ・差分回収・`Reap`を追加しました。既存メソッドは維持します。ノードの能力とプロキシの能力は別に確認してください。

`internal/networkpolicy` のベンチマークでは、1000ドメイン照合で毎回許可リストを走査してDNS名をIPとしてparseしていた処理を特定しました。コンパイル時の索引に変更し、従来のdomain/IP判定との一致を試験しています。1000件での測定は変更前65.9–181 µs、48112 B・1002 allocs/回から、変更後152–153 ns、0 B・0 allocs/回になりました（同一ホスト、3回測定、判定処理のみ。通信全体の速度ではありません）。測定コマンドは次の通りです。

```bash
go test ./internal/networkpolicy -run '^$' -bench . -benchmem -count=3
```

TUIログは追記描画に変更し、2000行・約4 MiBで区切ります。履歴全体を毎回再生成する処理をなくし、終了時にはpollingも停止します。
