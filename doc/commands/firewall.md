# `codex-dock firewall` — firewall 管理

> **日本語** | [English](../../en/commands/firewall.md)
>
> [← コマンドリファレンス一覧](../commands.md)

`codex-dock firewall` は `dock-net` 用の Linux `iptables` ルールを管理します。
ネットワーク作成（`codex-dock network`）とは責務が異なるため、運用時は分けて扱ってください。

---

## 実行前チェック

- Linux であること
- root 権限であること
- `iptables` がインストール済みであること

満たさない場合は Warning を表示して継続します。

---

## `firewall create` — ルール作成

```bash
codex-dock firewall create [--no-internet] [--proxy-container-url URL]
```

| オプション | 既定値 | 説明 |
|---|---|---|
| `--no-internet` | `false` | `dock-net` 作成時の IP Masquerade を無効化する（ネット遮断） |
| `--proxy-container-url` | `http://codex-auth-proxy:18080` | 許可対象の Auth Proxy URL |

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

次の項目を確認できます。

- Linux 対応可否
- root 実行可否
- `iptables` 検出
- `CODEX-DOCK` chain の有無
- `DOCKER-USER -> CODEX-DOCK` jump rule の有無
- `DOCKER-USER` 既定ポリシー
- `CODEX-DOCK` の最終 jump verdict

---

## `firewall rm` — ルール削除

```bash
codex-dock firewall rm
```

`DOCKER-USER -> CODEX-DOCK` の jump rule を削除し、`CODEX-DOCK` chain を削除します。

---

## 関連ドキュメント

- [firewall 仕様・運用ガイド](../firewall.md)
- [`codex-dock network` コマンド](network-cmd.md)
