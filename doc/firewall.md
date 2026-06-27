# firewall（廃止 / ルータモデルへ移行）

> **日本語** | [English](en/firewall.md)

> **このコマンドは廃止されました。** 旧 `codex-dock firewall`（Linux `iptables` の `DOCKER-USER` / `CODEX-DOCK` チェーン制御）と、関連フラグ `--allow-host` / `--block-host` / `--no-firewall` / `--sudo` は削除されました。

## 何が変わったか

ネットワーク隔離は **Docker のネイティブ機能のみ** で実現するようになり、`iptables` も `sudo` も不要になりました。

- 各ワーカーは専用の `Internal` ネットワーク（`dock-net-w-<name>`）に隔離され、ワーカー間は別 L2 セグメントのため疎通しません。
- `Internal: true` によりワーカーはホストにもインターネットにも直接到達できません。
- プロキシコンテナがルータとして唯一の egress 経路になります（HTTP CONNECT フォワードプロキシ + API リバースルート）。

詳細は [ネットワーク仕様](network.md) を参照してください。

## 旧機能の対応

| 旧（iptables） | 新（Docker ネイティブ） |
|---|---|
| `firewall create` で private/link-local を遮断 | `Internal` ネットワークで自動的にホスト/プライベート到達を遮断 |
| `--allow-host` で許可先追加 | 不要（egress はプロキシ経由のみ） |
| `--block-host` で遮断先追加 | `proxy run --forward-allow-domain` でドメイン allowlist を指定 |
| `--sudo` で root 権限実行 | 不要（Docker デーモンが処理） |
