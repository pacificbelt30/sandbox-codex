# `codex-dock firewall` — firewall 管理

> **日本語** | [English](../../en/commands/firewall.md)
>
> [← コマンドリファレンス一覧](../commands.md)

`codex-dock firewall` は `dock-net` 用の Linux `iptables` ルールを管理します。
ネットワーク作成（`codex-dock network`）とは責務が異なるため、運用時は分けて扱ってください。

---

## 実行前チェック

- Linux であること
- root 権限であること（または `--sudo` を付与）
- `iptables` がインストール済みであること

満たさない場合は Warning を表示して継続します。

> root でない場合は `--sudo` を付けると `iptables` 呼び出しのみ `sudo` 経由で実行します。
> 対話端末ではパスワードを一度だけ要求し、非対話環境（CI / TUI / `--detach`）では
> キャッシュ済み資格情報または NOPASSWD 設定を利用し、プロンプトで停止しません。

---

## `firewall create` — ルール作成

```bash
codex-dock firewall create [--no-internet] [--proxy-container-url URL] [--allow-host IP:PORT ...] [--block-host CIDR ...] [--sudo]
```

| オプション | 既定値 | 説明 |
|---|---|---|
| `--no-internet` | `false` | `dock-net` 作成時の IP Masquerade を無効化する（ネット遮断） |
| `--proxy-container-url` | `http://codex-auth-proxy:18080` | 許可対象の Auth Proxy URL |
| `--allow-host` | （なし） | 追加で許可する宛先 `IP:PORT`。繰り返し指定可。ホスト名ではなく IP リテラルを指定する（IPv6 は `[::1]:PORT`） |
| `--block-host` | （なし） | 追加で遮断する宛先 `CIDR` / `IP` / `IP:PORT`（IPv4）。繰り返し指定可。`--allow-host` の方が優先される |
| `--sudo` | `false` | root でないとき `iptables` 実行のみ `sudo` 経由にする。対話端末では一度だけパスワードを要求し、非対話環境では NOPASSWD/キャッシュ資格情報を利用 |

```bash
# 例: 社内レジストリ (203.0.113.10:5000) を許可しつつ firewall を作成
sudo codex-dock firewall create --allow-host 203.0.113.10:5000

# 例: 特定レンジ/ホストを追加で遮断
sudo codex-dock firewall create --block-host 203.0.113.0/24 --block-host 198.51.100.9:443

# 例: root でなく --sudo を使って適用（iptables 実行時のみパスワードを要求）
codex-dock firewall create --sudo --block-host 203.0.113.0/24

# run 時に直接指定することも可能
codex-dock run --agent claude --allow-host 203.0.113.10:5000 --block-host 203.0.113.0/24
```

### 動作概要

1. `DOCKER-USER` に `CODEX-DOCK` へのジャンプ導線を追加
2. Auth Proxy 向け許可ルールを優先挿入
3. private/link-local 宛を DROP
4. `RETURN` で終端

`dock-net` / `dock-net-proxy` が存在しない場合は、作成可否を対話で確認します。

---

## `firewall status` — 状態確認

```bash
codex-dock firewall status
```

先頭に `Firewall: Active / Not active / Unavailable` の1行判定を表示し、
有効化されていない場合は次に実行すべきコマンド（例: `sudo codex-dock firewall create`）を案内します。
続けて次の詳細項目を確認できます。

- Linux 対応可否
- root 実行可否
- `iptables` 検出
- `CODEX-DOCK` chain の有無
- `DOCKER-USER -> CODEX-DOCK` jump rule の有無
- `DOCKER-USER` 既定ポリシー
- `CODEX-DOCK` の最終 jump verdict

最後に `CODEX-DOCK` チェーンの**許可/遮断ルール一覧**を評価順に表示します。
どの宛先が `ALLOW`（通過）／`BLOCK`（遮断）されるか、`--allow-host` で追加した
許可先も含めて一目で確認できます。

```text
Rules (CODEX-DOCK chain, evaluated top to bottom):
  ALLOW  172.17.0.1/32       tcp/18080  auth proxy / allowed host
  ALLOW  203.0.113.10/32     tcp/5000   auth proxy / allowed host
  ALLOW  10.200.0.0/24       tcp/18080  dock-net subnet -> proxy
  BLOCK  10.0.0.0/8          all        private/link-local
  BLOCK  172.16.0.0/12       all        private/link-local
  BLOCK  192.168.0.0/16      all        private/link-local
  BLOCK  169.254.0.0/16      all        private/link-local
  BLOCK  127.0.0.0/8         all        private/link-local
  ALLOW  any                 all        default: hand back to Docker rules
```

---

## `firewall rm` — ルール削除

```bash
codex-dock firewall rm [--sudo]
```

`DOCKER-USER -> CODEX-DOCK` の jump rule を削除し、`CODEX-DOCK` chain を削除します。
削除も `iptables` 操作のため root が必要です。root でない場合は `--sudo` を付与してください。

---

## 関連ドキュメント

- [firewall 仕様・運用ガイド](../firewall.md)
- [`codex-dock network` コマンド](network-cmd.md)
