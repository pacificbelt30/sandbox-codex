# ネットワーク仕様（プロキシルータ + per-worker ネットワーク）

> **日本語** | [English](en/network.md)

codex-dock はネットワーク隔離を **Docker のネイティブ機能のみ** で実現します。`iptables` も `sudo` も使いません。隔離ルールは Docker デーモン（既に root 権限）が管理します。

---

## トポロジ（2つのプロキシ）

egress は役割で**2コンテナに分離**されています。**資格情報を持つ auth プロキシは一般通信を中継せず**、一般通信は資格情報を持たない egress プロキシだけが扱います（最小権限）。

```
                       ┌──────────── インターネット (public のみ)
        dock-net-proxy ─┤  ← 両プロキシの egress 用ブリッジ
                        │
        ┌───────────────┴───────────────┐
 ┌──────▼───────┐                ┌───────▼───────┐
 │ codex-auth-  │                │ codex-http-   │
 │ proxy        │                │ proxy         │
 │ :18080 reverse(/v1,/anthropic)│ :18082 forward(CONNECT/HTTP)
 │ :18081 admin                  │  + LAN遮断 + allowlist
 │ ★固定3ホストへのみ送信         │  ★資格情報なし
 └──┬─────────┬─┘                └──┬─────────┬──┘
    │ (両方が各ワーカー網にマルチホーム接続)  │
 dock-net-w-A …                   dock-net-w-A …
        │                               │
     worker A:  OPENAI_BASE_URL→auth / HTTP_PROXY→http
```

| ネットワーク | 種別 | 役割 |
|---|---|---|
| `dock-net-proxy` | bridge（NAT 有効） | **両プロキシ**の egress。ワーカーは接続しない |
| `dock-net-w-<name>` | bridge `Internal`（NAT 無効） | ワーカー専用。**両プロキシ**が追加接続される |

プロキシのロール:

| コンテナ | ロール | ポート | 役割 |
|---|---|---|---|
| `codex-auth-proxy` | `auth` | data 18080 / admin 18081 | リバースルート（`/v1`・`/anthropic`・`/chatgpt`）で**本物の資格情報を注入**。トークン発行・admin。**一般通信は中継しない（CONNECT/絶対URI は 405）** |
| `codex-http-proxy` | `egress` | 18082 | **フォワードプロキシ専用**（git/npm/pip）。資格情報なし。private/LAN 遮断・ドメイン allowlist |

- **ワーカー間遮断**: 各ワーカーは別々の `Internal` ネットワーク（別 L2 セグメント）にいるため相互に到達できません。
- **ワーカー→ホスト/インターネット遮断**: `Internal: true` のためホストルートも NAT もありません。唯一の到達先はプロキシです。
- **ワーカー→プロキシ**: Docker 埋め込み DNS で `codex-auth-proxy:18080`（API）と `codex-http-proxy:18082`（一般通信）にのみ到達します。`/admin/*` は auth プロキシの**egress 網 IP にバインドした別リスナー**で、ワーカー網からは到達できません（ホストの `127.0.0.1:18081` 経由のみ）。
- **egress の振り分け**: API（`OPENAI_/ANTHROPIC_BASE_URL`）は auth のリバースルート（資格注入）へ、一般通信（`HTTP(S)_PROXY`）は http のフォワードプロキシへ。`NO_PROXY=codex-auth-proxy,…` で API/トークンは直接届きます。
- **LAN 遮断**: `codex-http-proxy` は `--block-private` で private/loopback/link-local（RFC1918・127/8・**169.254/16=クラウドメタデータ**・ULA・CGNAT）への接続を拒否します（403）。`proxy run` は既定で有効。多層防御として auth プロキシの upstream dial にも適用。
- **直接（プロキシ非経由）の外向き通信はタイムアウトします**（設計どおり）。`HTTP(S)_PROXY` は `codex-dock run` が自動注入します。`--no-internet` 指定時は注入せず、auth の API ルートのみ到達可能。

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
- **`codex-dock stop`**: コンテナを停止するだけで、ネットワークは**意図的に残します**（再起動に必要なため）。削除は `rm` で行います。
- **`codex-dock rm <名前>` / TUI の削除（D）**: コンテナを削除し、専用ネットワークに残っている全エンドポイント（プロキシ含む）を強制切断してからネットワークを削除します。プロキシが起動していなくても確実に削除されます。
- **`--detach`（バックグラウンド）**: コンテナは残るので、`codex-dock rm` した時点でネットワークも削除されます。
- 自動生成されるワーカー名は、既存のコンテナ／ネットワークと衝突しないものを選びます（最終フォールバックのランダム接尾辞も含めて未使用を確認）。`--name` 指定時は Docker のコンテナ名の一意性により重複が拒否されます。

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

確認内容: auth `/health`、admin リスナーの `/admin/*`、データプレーンに `/admin/*` が出ないこと（分離）、**auth が一般通信を中継しないこと（405）**、egress のフォワード（HTTP / CONNECT）、**`--block-private` による LAN/loopback 遮断（403）**。コンテナ間隔離や `Internal` ネットワークの egress 遮断は Docker デーモンが必要なため、手動 E2E で確認してください。
