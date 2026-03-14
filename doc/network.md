# ネットワーク仕様 (dock-net)

> **日本語** | [English](en/network.md)

codex-dock は専用の Docker ブリッジネットワーク **dock-net** を使用してコンテナを隔離します。

---

## 現在のネットワーク / firewall 仕様

### dock-net 基本設定

| 項目 | 値 |
|---|---|
| ネットワーク名 | `dock-net` |
| ドライバー | `bridge` |
| ブリッジ名 | `dock-net0` |
| サブネット | `10.200.0.0/24` |
| ゲートウェイ | `10.200.0.1` |
| ICC | `false`（コンテナ間通信を無効化） |
| IP Masquerade | `true`（既定。`--no-internet` で `false`） |

### Linux の firewall ルール

`codex-dock` は Linux で `iptables` を使い、`DOCKER-USER` から `CODEX-DOCK` チェーンへジャンプするルールを管理します。

適用順序は次のとおりです。

1. （`dock-net-proxy0` が存在する場合）`DOCKER-USER` に NIC ベースの許可ルールを挿入
   - `-i dock-net0 -o dock-net-proxy0 -p tcp --dport <proxy-port> -j ACCEPT`
   - `-i dock-net-proxy0 -o dock-net0 -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT`
2. `DOCKER-USER` に `-i dock-net0 -j CODEX-DOCK` を挿入
3. `CODEX-DOCK` を flush
4. 許可ルールを追加
   - `--proxy-container-url` から解決できる `IP:PORT`
   - `--proxy-container-url` のポートに対する `dock-net` サブネット宛通信
5. private/link-local 宛 (`10/8`, `172.16/12`, `192.168/16`, `169.254/16`, `127/8`) を DROP
6. 最後に `RETURN`

> `codex-dock run` は起動時に firewall 適用を試行します。root 権限がない / `iptables` がない場合は Warning を表示して継続します。

---

## コマンド仕様

### `codex-dock firewall create`

```bash
codex-dock firewall create [--no-internet] [--proxy-container-url URL]
```

- Linux + root + `iptables` が利用可能な場合にルールを適用します。
- `dock-net` が存在しない場合は Warning を表示し、作成するか対話で確認します（作成しない場合は処理を中断）。
- `dock-net-proxy` が存在しない場合は Warning を表示し、作成するか対話で確認します。
- `--proxy-container-url` の既定値は `http://codex-auth-proxy:18080` です。

### `codex-dock firewall status`

```bash
codex-dock firewall status
```

次の状態を表示します。

- Linux 対応可否
- root 実行可否
- `iptables` 検出
- `CODEX-DOCK` chain の有無
- `DOCKER-USER -> CODEX-DOCK` jump rule の有無
- `DOCKER-USER` 既定ポリシー
- `CODEX-DOCK` の最終 jump verdict

### `codex-dock firewall rm`

```bash
codex-dock firewall rm
```

`DOCKER-USER -> CODEX-DOCK` の jump rule を削除し、`CODEX-DOCK` chain を削除します。

---

## 補足

- macOS / Windows (Docker Desktop) では Linux と同じ `iptables` 自動制御は行いません。
- `network create` は Docker ネットワーク作成のみで、firewall ルール作成は行いません。
