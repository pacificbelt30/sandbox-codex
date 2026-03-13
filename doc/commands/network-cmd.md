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
codex-dock firewall create [--no-internet]
```

> Linux の `iptables` ルールを適用します。
> root 権限がない場合や `iptables` 未導入の場合は Warning を表示して継続します。

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
dock-net status:
  ID:            a1b2c3d4e5f6789012345678
  Driver:        bridge
  Subnet:        10.200.0.0/24
  ICC:           disabled
  IP Masquerade: enabled
```

---

## 関連ドキュメント

- [ネットワーク仕様](../network.md) — dock-net の構成・セキュリティポリシー・トラブルシューティング
