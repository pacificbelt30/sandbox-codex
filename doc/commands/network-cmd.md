# `codex-dock network` — ネットワーク管理

> **日本語** | [English](../../en/commands/network-cmd.md)
>
> [← コマンドリファレンス一覧](../commands.md)

`dock-net` Docker ネットワークを管理します。
dock-net の仕様・セキュリティポリシーの詳細は [ネットワーク仕様](../network.md) を参照してください。

---

## `network create` — ネットワーク作成

```bash
codex-dock network create [--no-internet]
```

| オプション | 説明 |
|---|---|
| `--no-internet` | IP Masquerade を無効化してインターネットを遮断 |

> `codex-dock run` 実行時に自動的に作成されます。

---

## `firewall create` — firewall ルール作成

```bash
codex-dock firewall create [--no-internet] [--proxy-container-url URL]
```

> Linux の `iptables` ルールを適用します。
> root 権限がない場合や `iptables` 未導入の場合は Warning を表示して継続します。

## `firewall status` — firewall ルール状態確認

```bash
codex-dock firewall status
```

`dock-net` firewall の適用状態（Linux 対応可否、root 実行、iptables 検出、chain/jump rule、DOCKER-USER policy、CODEX-DOCK final jump）を表示します。

## `firewall rm` — firewall ルール削除

```bash
codex-dock firewall rm
```

`dock-net` 用に設定した firewall ルールを削除します。
root 権限がない場合や `iptables` 未導入の場合は Warning を表示して継続します。

---

## `network rm` — ネットワーク削除

```bash
codex-dock network rm
```

> 実行中のコンテナがある場合は先に停止してください。

---

## `network status` — ネットワーク状態確認

```bash
codex-dock network status
```

**出力例：**

```
dock-net ID:     a1b2c3d4e5f6
Driver:          bridge
ICC disabled:    true
IP Masquerade:   true
Subnet:          10.200.0.0/24
```

---

## 関連ドキュメント

- [ネットワーク仕様](../network.md) — dock-net の構成・セキュリティポリシー・トラブルシューティング
