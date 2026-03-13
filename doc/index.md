# codex-dock ドキュメント

> **日本語** | [English](en/index.md)

**codex-dock** は [Codex CLI](https://github.com/openai/codex) を Docker コンテナ内で安全に実行するための **AI サンドボックスコンテナマネージャー**です。
認証情報をコンテナから隔離する Auth Proxy、専用ブリッジネットワーク、並列ワーカー管理などを提供します。

---

## ドキュメント一覧

### はじめに

| ドキュメント | 内容 |
|---|---|
| [クイックスタート](getting-started.md) | インストールから初回実行まで |
| [アーキテクチャ概要](architecture.md) | システム全体の構成図・コンポーネント説明・起動シーケンス |
| [セキュリティ設計](security.md) | コンテナ設定・保護の仕組み・既知の問題 |

### Auth Proxy

| ドキュメント | 内容 |
|---|---|
| [Auth Proxy 概要・デプロイ](auth-proxy.md) | 認証プロキシの仕組み・起動方法・認証モード |
| [API エンドポイント仕様](auth-proxy/endpoints.md) | 全エンドポイントのリクエスト/レスポンス仕様 |
| [トークンの仕組みとセキュリティ](auth-proxy/tokens.md) | トークンライフサイクル・セキュリティ考慮事項 |
| [Auth Proxy のみを使う](proxy-standalone.md) | `codex-dock run` を使わずに Codex CLI を設定する方法 ✨ |

### ネットワーク

| ドキュメント | 内容 |
|---|---|
| [ネットワーク仕様](network.md) | dock-net の構成・セキュリティポリシー・トラブルシューティング |

### コマンドリファレンス

| ドキュメント | 内容 |
|---|---|
| [コマンドリファレンス（一覧）](commands.md) | 全コマンドのインデックス・グローバルオプション |
| [`codex-dock run`](commands/run.md) | コンテナ起動・承認モード・並列実行・パッケージ管理 |
| [`codex-dock proxy`](commands/proxy.md) | Auth Proxy の build / run / serve / stop / rm |
| [ワーカー管理 (ps / stop / rm / logs)](commands/worker.md) | コンテナの一覧・停止・削除・ログ表示 |
| [`codex-dock auth`](commands/auth.md) | 認証情報の show / set / rotate |
| [`codex-dock network`](commands/network-cmd.md) | dock-net の create / rm / status |
| [`codex-dock build`](commands/build.md) | サンドボックスイメージのビルド |
| [`codex-dock ui`](commands/ui.md) | TUI ダッシュボードのキーバインド |

### 設定

| ドキュメント | 内容 |
|---|---|
| [設定リファレンス](configuration.md) | config.toml の全設定項目・環境変数・認証ファイル |

---

## 全体像（一枚図）

```
┌─────────────────────────────────────────────────────────────┐
│  ホスト環境                                                   │
│                                                               │
│  ┌──────────────┐    ┌────────────────────────────────────┐  │
│  │  codex-dock  │    │  Auth Proxy (0.0.0.0:PORT)         │  │
│  │  (CLI)       │───▶│  - 短命トークン発行                │  │
│  └──────────────┘    │  - API キー / OAuth を保護          │  │
│         │            └──────────┬─────────────────────────┘  │
│         │                       │ host.docker.internal:PORT   │
│         ▼                       │                             │
│  ┌──────────────────────────────────────────────────────┐    │
│  │  dock-net (10.200.0.0/24)  Docker ブリッジネット     │    │
│  │                                                       │    │
│  │  ┌──────────────┐  ┌──────────────┐                  │    │
│  │  │ コンテナ A   │  │ コンテナ B   │  (ICC 無効)      │    │
│  │  │ codex-dock   │  │ codex-dock   │◀─ コンテナ間     │    │
│  │  │ worker-1     │  │ worker-2     │   通信ブロック   │    │
│  │  └──────────────┘  └──────────────┘                  │    │
│  └──────────────────────────────────────────────────────┘    │
│                              │ IP Masquerade                  │
│                              ▼                               │
│                        インターネット                         │
│                        (OpenAI API等)                         │
└─────────────────────────────────────────────────────────────┘
```

---

## 主な特徴

- **セキュリティ隔離**: Codex はホストではなく Docker コンテナ内で動作
- **Auth Proxy**: API キーがコンテナに直接渡らず、短命トークン経由で保護
- **dock-net**: ICC 無効・ホストアクセス制限付きの専用ブリッジネットワーク
- **git worktree**: 並列開発ブランチを独立したコンテナで実行
- **dock-ui**: 全ワーカーを一覧管理するターミナル UI
- **パッケージ管理**: `apt`, `pip`, `npm` パッケージを `--pkg` または `packages.dock` で指定

## 要件

- Go 1.24 以上
- Docker Engine 20.10 以上
