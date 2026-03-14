# `codex-dock network` — ネットワーク管理

> **日本語** | [English](../../en/commands/network-cmd.md)
>
> [← コマンドリファレンス一覧](../commands.md)

`dock-net` Docker ネットワークを管理します。

> Linux `iptables` firewall 管理は別コマンドです：[`codex-dock firewall`](firewall.md)

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

- [ネットワーク仕様](../network.md)
- [`codex-dock firewall` コマンド](firewall.md)
- [firewall 仕様・運用ガイド](../firewall.md)
