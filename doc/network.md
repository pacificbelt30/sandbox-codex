# ネットワーク仕様

- containerd/Kataの実行管理とネットワーク管理は分離する。
- root管理サービスが専用Linux bridge、network namespace、veth、nftables tableを所有し、Docker network APIやCNIの常駐サービスには依存しない。
- 詳細な[配置条件](kata.md)と[egressポリシー](proxy-policy.md)を参照。

## トポロジ

| network / endpoint | 構成 |
|---|---|
| `dock-net-proxy` | auth/egress proxyのprimary interface。ホストgatewayとNATを持つ |
| `dock-net-w-<name>` | ワーカーごとの独立L2 bridge。ホストIPv4・default route・NATを持たない |
| `codex-auth-proxy` | 必要なworker networkへ追加interfaceを接続。API reverseとtokenを提供 |
| `codex-http-proxy` | 一般通信を許可したworker networkだけへ追加接続。forward専用、実資格情報なし |

- ワーカーは1つの専用networkにだけ接続する。
- Kataワーカーに渡す認証・一般通信proxyのURLは、そのnetworkで検証済みのfront IPに固定し、guestのhostsファイルに依存しない。
- DNSは127.0.0.1。
- 外部名の解決とdialはproxy側が担当する。
- proxy namespaceのIP forwardingは無効化し、別worker網やprimary網へIP packetを中継しない。

- 専用nftables tableはworker bridgeからのホスト入力と他interfaceへの転送を落とす。
- 同一bridge内のworker/proxy通信は許可する。
- 外部向けNATを持つのはproxyのegress bridgeだけ。
- host IPv4 forwardingと既存firewallの方針は実行ホスト側で準備する。
- 本製品は他サービスのtableをflushしない。

## 管理ポートと資格情報

| proxy | listener | 公開範囲 |
|---|---|---|
| auth | data :18080 | worker front network |
| auth | admin :18081 | primary egress IP。hostでは127.0.0.1へ専用relayで公開 |
| egress | forward :18082 | 接続を許可したworker front network |

- adminはworker側IPへbindしない。
- hostの管理サービスsocketとnative containerd socketはguestへmountしない。
- APIキー/OAuth実tokenはauth proxyだけが保持し、egress proxyとworkerへ渡さない。

## 許可・拒否ルール

- 一般通信は `internal/networkpolicy` でdomain・IP/CIDR・portのallow/denyを評価する。
- HTTP、CONNECT、redirectへ同じ判定を適用し、DNS応答全体を検査したうえで選択したIPへdialを固定する。
- private/loopback/link-local/cloud metadata等は既定で拒否する。

```bash
oaiso proxy run \
  --forward-allow-domain github.com \
  --forward-allow-domain registry.npmjs.org \
  --forward-deny-domain example.invalid
```

- `--no-internet` はHTTP_PROXYを省くだけでなくegress proxyをworker networkへ接続しない。
- authの固定API経路は維持する。
- 詳細と設定優先順位は[プロキシポリシー](proxy-policy.md)を参照。

## 所有とライフサイクル

- networkのID・subnet・endpoint・namespace inode・bridge alias・firewall hashをroot private状態へ保存する。
- ネットワーク参照時は実bridgeと専用ルールの一致を検査し、未知peerを採用しない。
- 既存アドレスとVPN/static routeとの重複を避けて10.253.0.0/16から/24を予約する。

- `stop` は再起動用にnetworkとproxy接続を保持する。
- Dragonball再起動時は停止したworker側namespace/vethだけを同じIPで再生成し、残存tcfilterとの衝突を防ぐ。
- `rm` は正確なIDと選択済みproxyだけを切断して回収する。
- 未知peerや所有変更があれば削除を拒否し、journalを残す。

- 管理サービス再起動時は稼働中VM・namespaceを保持する。
- ホスト再起動で資源が消えた場合は、containerd taskが存在せずカーネルも空であることを確認し、保存済みIDと接続情報から復元する。
- 稼働中taskのnetwork欠落や所有権の変化は自動で採用しない。

```bash
oaiso network create
oaiso network status
oaiso network rm
```

- ワーカーのnetworkは通常の `run / stop / rm` に合わせて管理する。
- プロキシ単体は `scripts/smoke-proxy.sh`、VMを含めた通信分離は `TestKataAcceptance` で確認する。
- 旧 `scripts/e2e-docker.sh` は移行前Docker経路用の検証であり、現行構成の検証には使用しない。
