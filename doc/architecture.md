# アーキテクチャ概要

> **日本語** | [English](en/architecture.md)

codex-dock は大きく **4 つのコンポーネント**で構成されています。

---

## コンポーネント構成

```
codex-dock
├── cmd/                     CLIコマンド (cobra)
│   ├── run.go               コンテナ起動・ワーカー管理
│   ├── auth.go              認証設定 (auth set / auth status)
│   ├── build.go             サンドボックスイメージのビルド
│   ├── ps.go / stop.go / rm.go / logs.go
│   ├── ui.go                TUI ダッシュボード起動
│   └── network.go           dock-net の管理
│
├── internal/
│   ├── authproxy/           Auth Proxy（認証プロキシ）
│   │   ├── proxy.go         HTTPサーバ・トークン管理
│   │   └── auth.go          APIキー/OAuthクレデンシャル読み込み
│   ├── sandbox/             Docker コンテナ管理
│   │   ├── manager.go       コンテナのライフサイクル
│   │   ├── types.go         RunOptions 等の型定義
│   │   └── packages.go      パッケージ定義解析
│   ├── network/             dock-net 管理
│   │   └── manager.go       ブリッジネットワークの作成/削除
│   ├── worktree/            git worktree 管理
│   │   └── worktree.go      worktree の作成/削除
│   └── ui/                  ターミナル UI (Bubble Tea)
│       └── ui.go
│
└── docker/
    ├── Dockerfile           サンドボックスイメージ定義
    └── entrypoint.sh        コンテナ起動スクリプト（認証取得含む）
```

---

## 起動シーケンス

`codex-dock run` を実行した際の処理フローを示します。

```
ユーザー           codex-dock CLI          Auth Proxy              Docker / コンテナ
  │                    │                       │                         │
  │  codex-dock run    │                       │                         │
  │──────────────────▶│                       │                         │
  │                    │                       │                         │
  │                    │ 1. dock-net 確認/作成  │                         │
  │                    │──────────────────────────────────────────────▶ │
  │                    │                       │                         │
  │                    │ 2. Auth Proxy 起動     │                         │
  │                    │──────────────────────▶│                         │
  │                    │  (0.0.0.0:PORT)       │                         │
  │                    │                       │                         │
  │                    │ 3. 短命トークン発行    │                         │
  │                    │──────────────────────▶│                         │
  │                    │◀── cdx-xxxx...        │                         │
  │                    │                       │                         │
  │                    │ 4. コンテナ作成・起動  │                         │
  │                    │  CODEX_TOKEN=cdx-xxx  │                         │
  │                    │  OPENAI_BASE_URL=      │                         │
  │                    │  http://host.docker.  │                         │
  │                    │    internal:PORT/v1   │                         │
  │                    │  CODEX_REFRESH_TOKEN_ │                         │
  │                    │   URL_OVERRIDE=...    │                         │
  │                    │──────────────────────────────────────────────▶ │
  │                    │                       │                         │
  │                    │                       │  5. GET /token          │
  │                    │                       │◀────────────────────────│
  │                    │                       │  X-Codex-Token: cdx-xxx │
  │                    │                       │─────────────────────── ▶│
  │                    │                       │  {api_key or            │
  │                    │                       │   oauth_access_token}   │
  │                    │                       │                         │
  │                    │                       │  6. Codex CLI 起動      │
  │                    │                       │     (auth.json 生成済み) │
  │                    │                       │                         │
  │                    │                       │  7. POST /v1/responses  │
  │                    │                       │◀────────────────────────│
  │                    │                       │  ↓ 転送                 │
  │                    │                       │  api.openai.com/v1 or   │
  │                    │                       │  chatgpt.com/backend-api│
  │◀──────────────────────────────────────────────────────────────────── │
  │  コンテナ出力       │                       │                         │
```

---

## セキュリティ設計の原則

codex-dock のセキュリティは **「コンテナに秘密情報を直接渡さない」** という原則に基づいています。

```
                    NG (従来のアプローチ)
┌────────────┐                           ┌──────────────────┐
│   ホスト   │  OPENAI_API_KEY=sk-xxx   │    コンテナ      │
│            │─────────────────────────▶│  (危険: 漏洩リスク)│
└────────────┘                           └──────────────────┘

                    OK (codex-dock のアプローチ)
┌────────────────────────────────────────────────────────────┐
│   ホスト                                                    │
│                                                             │
│  API Key ──▶ Auth Proxy ──▶ プレースホルダー ──▶ コンテナ │
│  (保護)       (0.0.0.0)     (cdx-xxxx)        TTL付き      │
│               ↑ host.docker.internal:PORT で到達可能        │
│                   │                                         │
│                   │ API リクエスト中継時に                   │
│                   │ Authorization を本物に差し替え          │
│                   ▼                                         │
│              api.openai.com / chatgpt.com                   │
│                                                             │
│  ・実際の API Key / access_token はコンテナに渡らない       │
│  ・コンテナが保持するのはプレースホルダー (cdx-xxxx) のみ   │
│  ・プレースホルダーは OpenAI への直接アクセスに使えない     │
│  ・OAuth の refresh_token はホストのみが保持する            │
└────────────────────────────────────────────────────────────┘
```

---

## コンテナのセキュリティ設定

各サンドボックスコンテナには以下のセキュリティ設定が適用されます。

| 設定項目 | 値 | 効果 |
|---|---|---|
| `--cap-drop ALL` | すべての Linux ケーパビリティを削除 | 権限昇格・特権操作を防止 |
| `--security-opt no-new-privileges` | 新しい権限の取得を禁止 | setuid/setgid バイナリの悪用防止 |
| `USER codex (uid:1000)` | 非 root ユーザーで実行 | root 権限でのホスト操作を防止 |
| `--pids-limit 512` | 最大プロセス数を 512 に制限 | fork bomb 等の防止 |
| ネットワーク: `dock-net` | ICC 無効のブリッジネットワーク | コンテナ間通信のブロック |

---

## 認証モードの違い

codex-dock は **API キーモード** と **OAuth モード** の 2 つの認証方式をサポートします。

```
【API キーモード】
ホスト (Auth Proxy)                  コンテナ
  API キーをメモリ保持
  ↓
  CODEX_TOKEN=cdx-xxx        ──────▶ GET /token → {"api_key": "cdx-xxx"}  ← プレースホルダー
  OPENAI_BASE_URL=proxy/v1           export OPENAI_API_KEY=cdx-xxx  ← ダミー
                                     exec codex
                                     ↓
  POST /v1/responses ◀───────────────  Codex CLI (Authorization: Bearer cdx-xxx)
  Authorization を sk-xxx に差し替え  ← プロキシがインジェクト
  転送先: api.openai.com/v1

【OAuth モード (ChatGPT サブスクリプション)】
ホスト (Auth Proxy)                  コンテナ
  ~/.codex/auth.json
  access_token, id_token             CODEX_TOKEN=cdx-xxx
  refresh_token (ホスト内のみ) ─────▶ GET /token
                                       → {oauth_access_token: "cdx-xxx",  ← プレースホルダー
                                           id_token: "ey...",              ← 本物
                                           ...}
                                       write ~/.codex/auth.json
                                         access_token: "cdx-xxx"  ← ダミー
                                         refresh_token: ""
                                       write ~/.config/codex/config.toml
                                         chatgpt_base_url=proxy/chatgpt/
                                       exec codex
                                     ↓
  POST /v1/responses ◀───────────────  Codex CLI (Authorization: Bearer cdx-xxx)
  Authorization を本物の access_token に差し替え  ← プロキシがインジェクト
  ChatGPT-Account-Id も正値で上書き
  転送先: chatgpt.com/backend-api/codex
  ↓
  POST /oauth/token?cdx=xxx ◀────────  Codex CLI (8時間ごと)
  ホストの refresh_token を注入して
  auth.openai.com/oauth/token に転送
  access_token → "cdx-xxx" (プレースホルダー) に差し替えて返す
  (refresh_token は除外)
```

> **重要**: OAuth モードでは `refresh_token` も本物の `access_token` もコンテナに渡りません。
> コンテナが保持するのはプレースホルダー (`cdx-xxx`) のみで、これは OpenAI への直接アクセスに使えません。
> CODEX_TOKEN が失効すればプロキシ経由のリフレッシュも不可能になります。

---

## 実装状況サマリー

| カテゴリ | 実装済み | 部分実装 | 未実装 |
|---|---|---|---|
| 認証 (AUTH) | F-AUTH-01〜05, 07 | F-AUTH-06 | F-AUTH-08 |
| ネットワーク (NET) | F-NET-01, 03, 04, 05, 06 | F-NET-02 | — |
| パッケージ (PKG) | F-PKG-01〜04, 06 | F-PKG-05 | — |
| Worktree (WT) | F-WT-01〜04 | — | F-WT-05 |
| UI (UI) | F-UI-01 | F-UI-02, 03 | F-UI-04, 05 |
| セキュリティ (SEC) | NF-SEC-02, 03, 04 | NF-SEC-05 | NF-SEC-01, 06 |

> F-AUTH-04 (コンテナ停止時のトークン自動失効) は `sandbox.Manager.Stop()` が `proxy.RevokeToken()` を呼ぶことで解決済み。
> F-AUTH-07 (OAuth リフレッシュ中継) は `/oauth/token` プロキシエンドポイントで実現。
> F-NET-04 (Auth Proxy がコンテナから到達不能) は Auth Proxy を `0.0.0.0` にバインドし、ワーカーコンテナに `--add-host=host.docker.internal:host-gateway` を追加することで解決済み。コンテナは `http://host.docker.internal:PORT` 経由でプロキシに到達する。

詳細は [Auth Proxy 技術仕様](auth-proxy.md) および [ネットワーク仕様](network.md) を参照してください。
