# セキュリティ設計

> **日本語** | [English](en/security.md)

codex-dock のセキュリティ設計の原則・コンテナ設定・既知の制限をまとめています。

---

## 基本原則：コンテナに秘密情報を渡さない

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
│  (保護)                       (cdx-xxxx, TTL付き)           │
│               │                                             │
│               │ API リクエスト中継時に                       │
│               │ Authorization を本物に差し替え              │
│               ▼                                             │
│          api.openai.com / chatgpt.com                       │
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

## 多層防御の構造

```
【レイヤー 1: 認証情報の保護】
  API Key / OAuth トークン → Auth Proxy が保持
  コンテナには短命プレースホルダーのみ渡す
        │
        ▼
【レイヤー 2: プロセス権限の制限】
  --cap-drop ALL + no-new-privileges + 非 root 実行
  コンテナ内の悪意ある動作がホストに影響を与えにくい構造
        │
        ▼
【レイヤー 3: ネットワーク隔離】
  dock-net: ICC 無効、IP Masquerade でインターネットのみ許可
  コンテナ間の横断侵害を防止
        │
        ▼
【レイヤー 4: リソース制限】
  --pids-limit 512 で fork bomb 等を防止
```

---

## 実装済みの保護

| 保護 | 実装 | 詳細 |
|---|---|---|
| API キーの隔離 | ✅ | コンテナにはプレースホルダーのみ。本物のキーはプロキシがインジェクト |
| access_token の隔離 | ✅ | OAuth モードでも本物の access_token はコンテナに渡らない |
| refresh_token の保護 | ✅ | コンテナに渡さない。リフレッシュは `/oauth/token` 中継で実現 |
| 短命トークン | ✅ | TTL 付き、コンテナ停止時に即時失効 |
| API トラフィックの中継 | ✅ | `/v1/` と `/chatgpt/` のリバースプロキシで外部 API への直接通信を排除 |
| クレデンシャルのログ出力禁止 | ✅ | 認証情報を stdout/stderr に出力しない |
| `auth.json` の bind mount 禁止 | ✅ | コンテナ内の auth.json は access_token がプレースホルダーの安全なコピー |
| コンテナ間通信ブロック | ✅ | ICC 無効（`enable_icc=false`） |
| 権限昇格防止 | ✅ | `--cap-drop ALL` + `--security-opt no-new-privileges` |
| 非 root 実行 | ✅ | `USER codex (uid:1000)` |
| リソース制限 | ✅ | `--pids-limit 512` |

---

## 既知の問題・制限

| ID | 問題 | 影響度 | 詳細 |
|---|---|---|---|
| NF-SEC-01 | Auth Proxy が平文 HTTP 通信 | 高 | TLS または UNIX ソケットが未実装。同一ホスト上の Docker 内部通信のみで使用することを想定 |
| F-NET-02 | コンテナ→ホスト通信の遮断は Linux 依存 | 中 | Linux では `DOCKER-USER` + `iptables` で private/link-local 宛を遮断。root 権限が必要。macOS / Windows は未実装 |
| F-AUTH-06 | コンテナ ID による照合なし | 中 | トークンはコンテナ名と紐付けられているが、コンテナ ID との照合なし |

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

---

## 関連ドキュメント

- [Auth Proxy 技術仕様](auth-proxy.md) — クレデンシャル保護の実装詳細
- [Auth Proxy トークン仕様](auth-proxy/tokens.md) — セキュリティ考慮事項
- [ネットワーク仕様](network.md) — dock-net のセキュリティポリシー
- [アーキテクチャ概要](architecture.md) — 全体構成
