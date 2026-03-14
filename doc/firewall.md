# firewall 仕様・運用ガイド

> **日本語** | [English](en/firewall.md)

`codex-dock firewall` は Linux ホストで `iptables` の `DOCKER-USER` を制御し、
`dock-net` からの不要な到達先を制限するための専用コマンドです。

---

## このページの位置づけ

- `network` は「Docker ネットワークを作る機能」です。
- `firewall` は「通信を許可/拒否する機能」です。
- 実運用では **`network create` → `firewall create`** の順を推奨します。

---

## 対象環境と前提

`firewall` は次の条件で有効です。

- Linux ホスト
- root 権限で実行
- `iptables` コマンドが利用可能

前提を満たさない場合、`codex-dock` は Warning を表示して継続します。

---

## ルール適用の考え方

`codex-dock firewall create` は次の方針でルールを適用します。

1. `DOCKER-USER` から `CODEX-DOCK` へジャンプする導線を作る
2. Auth Proxy 向け通信のみ先に明示許可する
3. private/link-local 宛を拒否する
4. 最後に `RETURN` で他の Docker ルールへ戻す

### 許可される通信

- `--proxy-container-url` で指定した Auth Proxy の `IP:PORT`
- `dock-net` サブネット (`10.200.0.0/24`) から同ポートへの通信

### 拒否される通信

- private/link-local 宛: `10/8`, `172.16/12`, `192.168/16`, `169.254/16`, `127/8`

---

## 運用上の推奨フロー

```bash
# 1) ネットワーク作成
codex-dock network create

# 2) firewall 適用（root）
sudo codex-dock firewall create --proxy-container-url http://codex-auth-proxy:18080

# 3) 状態確認
sudo codex-dock firewall status
```

`firewall create` 実行時に `dock-net` / `dock-net-proxy` が存在しない場合は、
作成可否を対話で確認します（`Create <network> now? [y/N]:`）。

---

## トラブルシューティング

### Auth Proxy に接続できない

- `iptables -S DOCKER-USER` でルール順序を確認
- proxy 許可ルールより前で DROP されていないか確認
- `--proxy-container-url` のホスト名/ポート誤りを確認

### Linux 以外で `firewall` が効かない

- macOS / Windows (Docker Desktop) は Linux `iptables` 自動制御の対象外

---

## 関連ドキュメント

- [`codex-dock firewall` コマンドリファレンス](commands/firewall.md)
- [ネットワーク仕様 (dock-net)](network.md)
- [Auth Proxy のみを使う](proxy-standalone.md)
