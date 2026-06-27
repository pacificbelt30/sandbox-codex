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
- root 権限で実行（または `--sudo` を付与）
- `iptables` コマンドが利用可能

前提を満たさない場合、`codex-dock` は Warning を表示して継続します。

### root でない場合の `--sudo`

root で実行できない環境では、`run` / `firewall create` / `firewall rm` / `network rm`
に `--sudo` を付けると、**`iptables` の呼び出しのみ** `sudo` 経由で実行します
（codex-dock 本体は一般ユーザーのまま動くため、config や認証情報のパス解決は影響を受けません）。

- 対話端末では一度だけパスワードを要求します（内部的に `sudo -v`）。
- 非対話環境（CI / TUI / `--detach`）ではプロンプトで停止せず、キャッシュ済み資格情報または
  `NOPASSWD` 設定を利用します。いずれも無い場合は最初の `iptables` 呼び出しで明示的に失敗します。
- config の `firewall.sudo = true` でも同じ挙動を既定にできます（CLI の `--sudo` が優先）。

```bash
# root でなく --sudo で適用（iptables 実行時のみパスワードを要求）
codex-dock firewall create --sudo
codex-dock run --agent claude --sudo
```

---

## ルール適用の考え方

`codex-dock firewall create` は次の方針で `CODEX-DOCK` チェーンを
**上から順に**評価するよう適用します。

1. `DOCKER-USER` から `CODEX-DOCK` へジャンプする導線を作る
2. **許可 (RETURN)**: Auth Proxy ＋ `--allow-host` で追加した宛先
3. **遮断 (DROP)**: private/link-local ＋ `--block-host` で追加した宛先
4. 最後に `RETURN` で他の Docker ルールへ戻す

許可ルールが先に評価されるため、**`--allow-host` は `--block-host` より優先**されます。
末尾が `RETURN`（= private 以外の公開インターネットは素通り）なので、
公開 IP を遮断したい場合は `--block-host` を使います。

### 許可される通信

- `--proxy-container-url` で指定した Auth Proxy の `IP:PORT`
- `dock-net` サブネット (`10.200.0.0/24`) から同ポートへの通信
- `--allow-host IP:PORT`（または config `firewall.allow_hosts`）で追加した宛先

### 拒否される通信

- private/link-local 宛: `10/8`, `172.16/12`, `192.168/16`, `169.254/16`, `127/8`
- `--block-host CIDR|IP|IP:PORT`（または config `firewall.block_hosts`）で追加した宛先（IPv4）

### 許可/遮断を追加するカスタマイズ

```bash
# 社内レジストリ (203.0.113.10:5000) は許可、ある外部レンジ全体は遮断
sudo codex-dock firewall create \
  --allow-host 203.0.113.10:5000 \
  --block-host 203.0.113.0/24

# run でも同じフラグが使える
codex-dock run --agent claude --allow-host 203.0.113.10:5000 --block-host 198.51.100.9:443
```

毎回フラグを書く代わりに `~/.config/codex-dock/config.toml` に固定できます。

```toml
[firewall]
proxy_container_url = "http://codex-auth-proxy:18080"
allow_hosts = ["203.0.113.10:5000"]   # 常に許可
block_hosts = ["203.0.113.0/24"]      # 常に遮断
```

> `--block-host` は IPv4 の `CIDR` / `IP` / `IP:PORT` を受け付けます。
> `IP` 単体は `/32`、`IP:PORT` は TCP の該当ポートのみ遮断します。
> 適用状況は `codex-dock firewall status` の `ALLOW` / `BLOCK` 一覧で確認できます
> （`--block-host` 由来は `custom block` と表示）。

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
