# `codex-dock auth` — 認証管理

> **日本語** | [English](../../en/commands/auth.md)
>
> [← コマンドリファレンス一覧](../commands.md)

API キーや OAuth の認証情報を管理します。

---

## `auth show` — 認証状態の確認

```bash
codex-dock auth show
```

現在の認証ソースを表示します（実際のキーやトークンは表示されません）。

**出力例（API キーの場合）：**

```
Auth source: OPENAI_API_KEY env
Configured:  yes
```

**出力例（OAuth の場合）：**

```
Auth source: ~/.codex/auth.json (OAuth/ChatGPT subscription)
Configured:  yes
```

**出力例（未設定の場合）：**

```
Auth source: none
Configured:  no
```

---

## `auth set` — API キーの保存

```bash
export OPENAI_API_KEY=sk-...
codex-dock auth set
```

`OPENAI_API_KEY` 環境変数の値を `~/.config/codex-dock/apikey` に保存します。
パーミッションは `0600` で保護されます。

---

## `auth rotate` — トークンのローテーション

```bash
codex-dock auth rotate
```

現在発行中の全トークンを無効化します。

---

## 認証ファイルの場所

| ファイル | 場所 | 説明 |
|---|---|---|
| API キー | `~/.config/codex-dock/apikey` | `codex-dock auth set` で保存 |
| OAuth クレデンシャル | `~/.codex/auth.json` | Codex CLI が生成（`codex login` で作成） |

詳細は [設定リファレンス — 認証ファイル](../configuration.md#認証ファイルの場所) を参照してください。

---

## 関連ドキュメント

- [Auth Proxy 技術仕様](../auth-proxy.md) — クレデンシャルの優先順位と保護の仕組み
- [設定リファレンス](../configuration.md) — 認証ファイルの形式
- [クイックスタート](../getting-started.md) — 認証の初期設定手順
