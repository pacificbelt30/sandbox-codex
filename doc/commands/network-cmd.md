# `codex-dock network` — ネットワーク管理

> **日本語** | [English](../en/commands/network-cmd.md)
>
> [← コマンドリファレンス一覧](../commands.md)

egress ネットワーク（`dock-net-proxy`、プロキシのインターネット到達用）を管理します。ワーカー用の `Internal` ネットワークはワーカーのライフサイクルに合わせて自動で作成・削除されるため、手動管理は不要です。

> 旧 `codex-dock firewall`（iptables）は廃止されました。隔離は Docker ネットワークで実現します。

---

## `network create` — egress ネットワーク作成

```bash
codex-dock network create
```

> `codex-dock proxy run` 実行時にも未作成なら自動的に作成されます。

---

## `network rm` — egress ネットワーク削除

```bash
codex-dock network rm
```

> プロキシコンテナが接続中の場合は先に停止・削除してください。

---

## `network status` — ネットワーク状態確認

```bash
codex-dock network status
```

**出力例：**

```
Egress network:  dock-net-proxy
  ID:            a1b2c3d4e5f6
  Driver:        bridge
  Internal:      false
  Subnet:        172.20.0.0/16
Worker networks: 2 (Internal, one per worker)
  - dock-net-w-codex-brave-otter
  - dock-net-w-codex-calm-finch
```

---

## 関連ドキュメント

- [ネットワーク仕様](../network.md)
