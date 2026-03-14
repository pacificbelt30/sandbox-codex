# ネットワーク仕様 (dock-net)

> **日本語** | [English](en/network.md)

codex-dock は専用の Docker ブリッジネットワーク **dock-net** を使用してコンテナを隔離します。

---

## dock-net 基本設定

| 項目 | 値 |
|---|---|
| ネットワーク名 | `dock-net` |
| ドライバー | `bridge` |
| ブリッジ名 | `dock-net0` |
| サブネット | `10.200.0.0/24` |
| ゲートウェイ | `10.200.0.1` |
| ICC | `false`（コンテナ間通信を無効化） |
| IP Masquerade | `true`（既定。`--no-internet` で `false`） |

---

## ネットワーク管理コマンド

### `codex-dock network create`

```bash
codex-dock network create [--no-internet]
```

- `dock-net` を作成します。
- `--no-internet` を指定すると、IP Masquerade を無効化してインターネットアクセスを遮断します。
- `codex-dock run` 実行時には `dock-net` がなければ自動作成されます。

### `codex-dock network status`

```bash
codex-dock network status
```

`dock-net` の現在状態（driver / subnet / ICC / IP Masquerade）を表示します。

### `codex-dock network rm`

```bash
codex-dock network rm
```

`dock-net` を削除します。利用中コンテナがある場合は先に停止してください。

---

## firewall との関係

- `network create` は Docker ネットワーク作成のみを担当します。
- Linux `iptables` を使った通信制御（`codex-dock firewall`）は別機能です。
- 実運用では `network` 作成後に firewall を設定することを推奨します。

firewall の詳細は専用ページを参照してください。

- [firewall 仕様・運用ガイド](firewall.md)
- [`codex-dock firewall` コマンドリファレンス](commands/firewall.md)

---

## 補足

- macOS / Windows (Docker Desktop) では Linux の `iptables` と同等の自動制御は行いません。
