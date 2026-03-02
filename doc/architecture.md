# アーキテクチャ概要

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
ユーザー                codex-dock CLI            Auth Proxy            Docker / コンテナ
  │                        │                          │                       │
  │  codex-dock run        │                          │                       │
  │───────────────────────▶│                          │                       │
  │                        │                          │                       │
  │                        │ 1. dock-net の確認/作成  │                       │
  │                        │─────────────────────────────────────────────────▶│
  │                        │                          │                       │
  │                        │ 2. Auth Proxy 起動        │                       │
  │                        │─────────────────────────▶│                       │
  │                        │  (127.0.0.1:PORT でListen)│                       │
  │                        │                          │                       │
  │                        │ 3. 短命トークン発行要求   │                       │
  │                        │─────────────────────────▶│                       │
  │                        │◀─ token: "cdx-xxxx..."  ─┤                       │
  │                        │                          │                       │
  │                        │ 4. コンテナ作成・起動    │                       │
  │                        │  env: CODEX_TOKEN=...    │                       │
  │                        │  env: CODEX_AUTH_PROXY_URL=...                   │
  │                        │─────────────────────────────────────────────────▶│
  │                        │                          │                       │
  │                        │                          │  5. /token エンドポイント呼び出し
  │                        │                          │◀──────────────────────┤
  │                        │                          │  Header: X-Codex-Token │
  │                        │                          │─────────────────────▶│
  │                        │                          │  (api_key or           │
  │                        │                          │   oauth_access_token)  │
  │                        │                          │                       │
  │                        │                          │  6. Codex CLI 起動    │
  │                        │                          │  (認証情報は env or    │
  │◀───────────────────────────────────────────────────────────────────────── │
  │  コンテナ出力           │                          │  ~/.codex/auth.json)   │
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
│  API Key ──▶ Auth Proxy ──▶ 短命トークン ──▶ コンテナ     │
│  (保護)       (127.0.0.1)    (cdx-xxxx)     TTL付き        │
│                                                             │
│  ・実際の API Key はコンテナに渡らない                      │
│  ・トークンには有効期限 (TTL) がある                         │
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
ホスト                          コンテナ (entrypoint.sh)
OPENAI_API_KEY=sk-xxx
      │
      ▼
Auth Proxy                      CODEX_TOKEN=cdx-xxx
(メモリ内に API キー保持)  ──▶  /token GET
                                 ◀── {"api_key": "sk-xxx"}
                                 export OPENAI_API_KEY=sk-xxx
                                 exec codex

【OAuth モード (ChatGPT サブスクリプション)】
ホスト                          コンテナ (entrypoint.sh)
~/.codex/auth.json
  access_token: "ey..."         CODEX_TOKEN=cdx-xxx
  refresh_token: "rt-..."  ──▶  /token GET
      │                         ◀── {"oauth_access_token": "ey..."}
      │ refresh_token は         write ~/.codex/auth.json
      │ ホスト側のみ保持!        { "access_token": "ey..." }
      ▼                         exec codex
 (コンテナには渡さない)
```

> **重要**: OAuth モードでは `refresh_token` はコンテナに渡りません。
> コンテナが侵害されても、攻撃者はトークンを更新できません。

---

## 実装状況サマリー

| カテゴリ | 実装済み | 部分実装 | 未実装 |
|---|---|---|---|
| 認証 (AUTH) | F-AUTH-01〜03, 05 | F-AUTH-06, 07 | F-AUTH-04, 08 |
| ネットワーク (NET) | F-NET-01, 03, 05, 06 | F-NET-02 | F-NET-04 |
| パッケージ (PKG) | F-PKG-01〜04, 06 | F-PKG-05 | — |
| Worktree (WT) | F-WT-01〜04 | — | F-WT-05 |
| UI (UI) | F-UI-01 | F-UI-02, 03 | F-UI-04, 05 |
| セキュリティ (SEC) | NF-SEC-02, 03, 04 | NF-SEC-05 | NF-SEC-01, 06 |

詳細は [Auth Proxy 技術仕様](auth-proxy.md) および [ネットワーク仕様](network.md) を参照してください。
