# プロキシの通信ポリシーと拡張

ワーカーの一般通信は認証情報を持たないegress proxyが扱う。OpenAI/Anthropicの固定認証経路とは別で、下記ルールによってAI APIの固定経路を許可・拒否するものではない。ワーカーからの直接通信や他ワーカーへの通信は専用bridge・namespace・nftablesで遮断する。

## 設定

`~/.config/codex-dock/config.toml`:

```toml
[proxy]
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
codex-dock proxy run \
  --forward-allow-domain github.com \
  --forward-deny-domain blocked.github.com \
  --forward-allow-port 80,443 \
  --forward-deny-cidr 203.0.113.0/24
```

判定規則:

- 拒否ルールを優先する。許可リストへの追加で拒否を打ち消すことはできない。
- ドメインは大文字小文字を区別せず、指定した名前とその全サブドメインに一致する。`*.example.com`やURLではなく`example.com`を指定する。国際化ドメインはASCIIのPunycode表記を使う。
- 各許可リストが空ならその種類の追加制限はない。複数種類を指定した場合、ドメイン・接続IP・ポートのすべてが一致する必要がある。全許可リストが空の場合は、拒否リストとprivate IP遮断のみが適用される。
- 管理するプロキシはprivate/loopback/link-local/metadata/CGNATへの接続を常に拒否する。CIDR許可リストでこれを解除できない。
- ドメインとポートはDNS問い合わせ前に検証する。DNS応答に拒否IPが一つでも含まれれば接続しない。確認済みIPへ直接接続し、接続時に名前を再解決しない。
- HTTPリダイレクト先も同じ判定を行う。CONNECTは要求された接続先ホスト・ポート・接続IPを検査する。TLS内のURLやSNIを復号・検査する機能ではない。
- ドメイン拒否は要求された名前への制限で、同じサービスへのIP直指定や別名を一括で拒否する保証ではない。厳密な接続先制限にはドメイン許可リストと、必要に応じてCIDR/ポートの制限を組み合わせる。
- 一般通信用proxyは環境変数の`HTTP_PROXY`等へ接続を委譲しない。上流proxyを追加する場合は、接続先ポリシーとの整合性を保つ専用transportが必要。

## 更新と既存環境からの移行

ルールは起動時に検証して固定する。未知のロール、誤ったドメイン/CIDR/ポートは起動エラー。ホットリロードとワーカーごとの個別ルールは現時点では提供しない。共有egress proxyに接続した全ワーカーへ同じルールが適用される。

`proxy run`は既存egressコンテナのポリシー識別子と要求した設定を照合し、古い版や設定の不一致をエラーにする。既存コンテナへ設定を反映したことにはせず、稼働中プロキシも自動で削除しない。設定ファイルの編集だけでは稼働中プロキシは変化しない。

1. 稼働ワーカーを停止し、成果を保存する。
2. 現行バイナリで `codex-dock proxy build` を実行する。
3. `codex-dock proxy stop --name codex-http-proxy`、`codex-dock proxy rm --name codex-http-proxy` で一般通信用proxyを回収する。
4. `codex-dock proxy run` で新しい設定の一般通信用proxyを作成する。
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

将来、ワーカー別認可を追加する場合は、認証済みワーカーidentityをポリシーへ渡す経路と接続プール/CONNECTの寿命を合わせて設計する。未検証のHTTPヘッダーや申告されたワーカー名を認可根拠にしない。

## 動作確認

宛先判定・DNS・redirect・CONNECTは自動テスト、VMを含めた通信分離は実Kata受入試験で確認しています。
詳細は[テストと動作確認](testing.md)を参照してください。
