# ネットワーク仕様（プロキシルータ + per-worker ネットワーク）

> **日本語** | [English](en/network.md)

codex-dock はネットワーク隔離を **Docker のネイティブ機能のみ** で実現します。`iptables` も `sudo` も使いません。隔離ルールは Docker デーモン（既に root 権限）が管理します。

---

## トポロジ

```
                         ┌──────────────── インターネット (NAT/masquerade ON)
          dock-net-proxy ─┤  ← プロキシの egress 用ブリッジ
                          │
                  ┌───────┴────────┐
                  │ codex-auth-    │  各ワーカー網へマルチホーム接続
                  │ proxy (router) │  data-plane :18080 / admin :18081
                  └─┬───────────┬──┘
   Internal          │           │          Internal
 dock-net-w-A ───────┘           └───────── dock-net-w-B
   (NAT 無効 / ホストルート無し)        (別 L2 セグメント)
       │                                  │
   worker A                            worker B   ← 相互疎通不可
```

| ネットワーク | 種別 | 役割 |
|---|---|---|
| `dock-net-proxy` | bridge（NAT 有効） | プロキシの egress（インターネット到達）。ワーカーは接続しない |
| `dock-net-w-<name>` | bridge `Internal`（NAT 無効） | ワーカー専用。プロキシのみが追加接続される |

- **ワーカー間遮断**: 各ワーカーは別々の `Internal` ネットワーク（別 L2 セグメント）にいるため相互に到達できません。
- **ワーカー→ホスト/インターネット遮断**: `Internal: true` のためホストルートも NAT もありません。唯一の到達先はプロキシです。
- **ワーカー→プロキシ**: 同一ネットワーク上の Docker 埋め込み DNS（`codex-auth-proxy`）でデータプレーンポート（18080）にのみ到達します。`/admin/*`（トークン発行など）は別リスナーで、しかも**プロキシの egress 網 IP にバインド**されるため、ワーカー網（別サブネット）からは到達できません（接続拒否）。ホストからは公開ポート `127.0.0.1:18081` 経由でのみ到達します。
- **egress はすべてプロキシ経由**: 一般通信（git/npm/pip/curl）は `HTTP(S)_PROXY` 経由でプロキシの HTTP CONNECT フォワードプロキシへ、OpenAI/Anthropic API は認証注入を行うリバースルートへ流れます。
- **直接（プロキシ非経由）の外向き通信はタイムアウトします**。これは設計どおりの挙動です。ワーカー網は `Internal` でホストルートも NAT も無いため、`HTTP(S)_PROXY` を尊重しない通信や `--no-internet` 指定時は外に出られません。`HTTP(S)_PROXY` は `codex-dock run` が自動注入します。

---

## ネットワーク管理コマンド

### `codex-dock network create`
egress ネットワーク（`dock-net-proxy`）を作成します。`proxy run` も未作成なら自動作成します。

### `codex-dock network status`
egress ネットワークの状態と、現在存在する per-worker ネットワーク一覧を表示します。

### `codex-dock network rm`
egress ネットワークを削除します。per-worker ネットワークはワーカー削除時に自動で切断・削除されます。

### per-worker ネットワークのライフサイクル
- **フォアグラウンドの `codex-dock run`**（`--detach` なし）: 終了時にコンテナと専用ネットワークを自動削除します。ネットワークが蓄積しません。残したい場合は `--keep` を付けます。
- **`--detach`（バックグラウンド）**: コンテナは残るので、`codex-dock rm <名前>` で削除した時点でネットワークも切断・削除されます。
- 自動生成されるワーカー名は、既存のコンテナ／ネットワークと衝突しないものを選びます（万一の衝突でワーカー同士が同じネットワークを共有することを防止）。

---

## egress 制御（フォワードプロキシ allowlist）

`codex-dock proxy run --forward-allow-domain <domain>`（繰り返し可）でフォワードプロキシの到達先ドメインを制限できます。指定したドメインとそのサブドメインのみ許可され、その他は 403 になります。未指定なら全許可です。

```bash
codex-dock proxy run \
  --forward-allow-domain github.com \
  --forward-allow-domain registry.npmjs.org \
  --forward-allow-domain pypi.org
```

`codex-dock run --no-internet` を付けると、そのワーカーには `HTTP(S)_PROXY` を注入しません（API リバースルートのみ到達可能で、一般 egress は無効）。

---

## 補足

- iptables を使わないため、**macOS / Windows (Docker Desktop) でも Linux と同等** の隔離が得られます（`Internal` ネットワークの遮断ルールは Docker Desktop が管理）。
- 旧 `codex-dock firewall` コマンドと `--allow-host`/`--block-host`/`--no-firewall`/`--sudo` フラグは廃止されました。

## 動作確認

Docker 不要でプロキシ／ルータの主要動作を確認できるスモークテストを同梱しています（要 `go` / `python3` / `curl`）。

```bash
bash scripts/smoke-proxy.sh
```

確認内容: データプレーン `/health`、admin リスナーの `/admin/*`、データプレーンポートに `/admin/*` が出ないこと（分離）、フォワードプロキシ（HTTP / CONNECT）、ドメイン allowlist による 403。コンテナ間隔離や `Internal` ネットワークの egress 遮断は Docker デーモンが必要なため、`doc/network.md` の手動 E2E 手順で確認してください。
