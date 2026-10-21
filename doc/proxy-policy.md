# プロキシの通信ポリシーと拡張

ワーカーの一般通信は認証情報を持たないegress proxyが扱う。OpenAI/Anthropicの固定認証経路とは別で、下記ルールによってAI APIの固定経路を許可・拒否するものではない。ワーカーからの直接通信や他ワーカーへの通信は専用bridge・namespace・nftablesで遮断する。

## 設定

`~/.config/codex-dock/config.toml`:

```toml
[proxy]
forward_default_deny = true
forward_allow_domains = ["github.com", "npmjs.org", "pypi.org", "pythonhosted.org"]
forward_deny_domains = ["blocked.github.com"]
forward_allow_ports = [80, 443]
forward_deny_ports = [25]
# IP単体、IPv4/IPv6のCIDRを指定可能。DNSが返すIPにも適用する。
forward_allow_cidrs = []
forward_deny_cidrs = []
```

`proxy run`には同名の単数形フラグがある。ドメイン・CIDRは繰り返し指定、ポートは繰り返しまたはカンマ区切りで指定する。明示したフラグは対応する設定ファイル項目を上書きする。

```bash
cxd proxy run \
  --forward-allow-domain github.com \
  --forward-deny-domain blocked.github.com \
  --forward-allow-port 80,443 \
  --forward-deny-cidr 203.0.113.0/24
```

判定規則:

- 拒否ルールを優先する。許可リストへの追加で拒否を打ち消すことはできない。
- ドメインは大文字小文字を区別しない。`example.com` はその名前と全サブドメイン、`=api.example.com` はその名前だけに一致する。許可・拒否の両方で使える。`*.example.com`やURLは指定しない。国際化ドメインはASCIIのPunycode表記を使う。
- `forward_default_deny = true`（CLI `--forward-default-deny`）では、ドメインとCIDRの許可リストが両方とも空なら一般通信を拒否する。ポートだけの指定では宛先を許可しない。固定AI経路の認証は独立している。
- `forward_default_deny` がfalseまたは省略の場合、空の許可リストはその種類の追加制限を加えない。全許可リストが空でも公開宛て通信を許可する従来動作であり、「空＝通信禁止」ではない。複数種類を指定した場合は、ドメイン・接続IP・ポートのすべてを満たす必要がある。
- 管理するプロキシはprivate/loopback/link-local/metadata/CGNATへの接続を常に拒否する。CIDR許可リストでこれを解除できない。
- ドメインとポートはDNS問い合わせ前に検証する。DNS応答に拒否IPが一つでも含まれれば接続しない。確認済みIPへ直接接続し、接続時に名前を再解決しない。
- HTTPリダイレクト先も同じ判定を行う。CONNECTは要求された接続先ホスト・ポート・接続IPを検査する。TLS内のURLやSNIを復号・検査する機能ではない。
- ドメイン拒否は要求された名前への制限で、同じサービスへのIP直指定や別名を一括で拒否する保証ではない。厳密な接続先制限にはドメイン許可リストと、必要に応じてCIDR/ポートの制限を組み合わせる。
- 一般通信用proxyは環境変数の`HTTP_PROXY`等へ接続を委譲しない。上流proxyを追加する場合は、接続先ポリシーとの整合性を保つ専用transportが必要。

## 更新と既存環境からの移行

ルールは起動時に検証して固定する。未知のロール、誤ったドメイン/CIDR/ポートは起動エラー。ホットリロードは提供しない。通常のワーカーは共有egress proxyを使用する。`cxd run --egress-allow-domain api.example.com` を指定したワーカーは、ポリシーの識別子を持つ専用egress proxyに接続する。異なる許可リストのワーカーはproxyを共有しない。同じ許可リストのワーカーは共有できる。プロジェクトの `codex-dock.toml` では `[run] egress_allow_domains = ["api.example.com"]` と書ける。ホストのグローバル許可ドメインがある場合、ワーカーの指定はその範囲内に限る。`--no-internet` は一般通信を止める。

許可ルールの変更は新しいproxyと新しいワーカーに適用される。設定編集だけでは既存のCONNECTや通信中のストリームを打ち切らない。緊急時は `cxd policy quarantine --worker <ID>` で対象ワーカーを停止・隔離する。一般通信だけでなくAI通信と全ジョブも終了し、再起動は禁止される。workspace・worktree・ログは保持する。新しいルールで再開するには新しいワーカー・要求IDを使う。隔離の詳細と失敗時の扱いは[追加制御](sandbox-features.md#通信の停止と隔離)を参照。不要になったポリシーproxyは、利用ワーカーを停止した後に `cxd proxy rm --name <proxy名> --force` で回収できる。

既定拒否はTUIの `Default deny` でも選べる。CLIでの新規ワーカー作成時に既定拒否で宛先が未許可なら、一般egressへ接続しない。Go APIでは `Options.EgressPolicy.DefaultDeny` を使う。ホスト側に許可が一つもない場合、ワーカー側から許可ドメインを追加して緩めることはできない。
Go APIのホストポリシーは `LaunchRequest.EgressAllowDomains` を省略しても管理proxyで強制する。`=api.example.com` のホスト規則をワーカーが `api.example.com` に広げることも拒否する。設定内容は不変の要求条件に含まれ、同じRequestIDへ異なるポリシーを送ると競合する。

`proxy run`は既存egressコンテナのポリシー識別子と要求した設定を照合し、古い版や設定の不一致をエラーにする。既存コンテナへ設定を反映したことにはせず、稼働中プロキシも自動で削除しない。設定ファイルの編集だけでは稼働中プロキシは変化しない。

既定のproxyイメージはCLIのビルド版とprotocolラベルを照合し、古い場合はソースcheckoutからBuildKitで再ビルドする。独自の `proxy_image` は自動で上書きせず、互換性が不明なら明示的な再ビルドを要求する。

1. 稼働ワーカーを停止し、成果を保存する。
2. 現行バイナリで `cxd proxy build` を実行する。
3. `cxd proxy stop --name codex-http-proxy`、`cxd proxy rm --name codex-http-proxy` で一般通信用proxyを回収する。
4. `cxd proxy run` で新しい設定の一般通信用proxyを作成する。
5. ワーカーを新規作成する。KataワーカーはproxyのIPを固定しており、IP変更時は保存済みワーカーの再起動を拒否する。

改造版proxyではトークンと既存認証protocolを維持する。auth proxyも更新する場合は、同じ停止期間に[更新手順](operations.md#cliプロキシを更新する)を適用する。

## コードの境界

| モジュール | 責任 |
|---|---|
| `internal/networkpolicy` | HTTP/実行エンジン/認証から独立した宛先判定、設定検証、DNS応答検証とIP固定dialer |
| `authproxy/forward.go` | 一般通信のHTTP/CONNECT処理 |
| `authproxy/transport.go` | ポリシーとHTTP transportの接続、HTTPリダイレクトの再検証 |
| `authproxy/openai.go`, `anthropic.go` | 各プロバイダーの認証注入とOAuth更新 |
| `authproxy/reverse.go`, `websocket.go` | 共通のHTTP/SSE転送とWebSocket処理 |
| `authproxy/tokens.go`, `admin.go`, `restore.go` | placeholderの発行・失効・復元と管理API |
| `authproxy/config.go`, `proxy.go` | 設定とサービスの組み立て・起動停止 |
| `authproxy/service.go`, `remote.go` | ワーカー管理側との境界。HTTP上の管理protocolを保てば外部実装へ交換可能 |
| `cmd/proxy_policy.go` | CLI/configの読み込みと既存コンテナの設定照合 |

独自ルールを加える場合は`networkpolicy.Policy`の`CheckDestination`と`CheckAddress`を実装し、`authproxy.Config.EgressPolicy`へ注入する。設定由来の基本ポリシーとANDで合成するため、追加実装がprivate IP遮断等を緩めることはない。拒否には`networkpolicy.ErrDenied`を返す。実装は並行呼び出しに対応させ、存続する接続との不整合を避けるためポリシーを稼働中に変更しない。

ポリシーはワーカー固有のinternal networkへ接続したproxyの選択で強制する。未検証のHTTPヘッダーや申告されたワーカー名を認可根拠にしない。

## 動作確認

宛先判定・DNS・redirect・CONNECTは自動テスト、VMを含めた通信分離は実Kata受入試験で確認しています。
詳細は[テストと動作確認](testing.md)を参照してください。

[TUI・プリセット・検査・拒否イベント](sandbox-features.md#通信の確認と設定)
