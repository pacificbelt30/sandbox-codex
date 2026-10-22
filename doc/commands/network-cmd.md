# `oaiso network` — ネットワーク管理

> **日本語** | [English](../en/commands/network-cmd.md)
>
> [← コマンドリファレンス一覧](../commands.md)

egress ネットワーク（`dock-net-proxy`、プロキシのインターネット到達用）を管理します。ワーカー用の `Internal` ネットワークはワーカーのライフサイクルに合わせて自動で作成・削除されるため、手動管理は不要です。

> 旧 `oaiso firewall`（iptables）は廃止されました。隔離はroot管理サービスの専用bridge・namespace・nftablesで実現します。

---

## `network create` — egress ネットワーク作成

```bash
oaiso network create
```

> `oaiso proxy run` 実行時にも未作成なら自動的に作成されます。

---

## `network rm` — egress ネットワーク削除

```bash
oaiso network rm
```

> プロキシコンテナが接続中の場合は先に停止・削除してください。

---

## `network status` — ネットワーク状態確認

```bash
oaiso network status
```

**出力例：**

```
Egress network:  dock-net-proxy
  ID:            a1b2c3d4e5f6
  Driver:        bridge
  Internal:      false
  Subnet:        10.253.0.0/24
Worker networks: 2 (Internal, one per worker)
  - dock-net-w-codex-brave-otter
  - dock-net-w-codex-calm-finch
```

---

## 関連ドキュメント

- [ネットワーク仕様](../network.md)
