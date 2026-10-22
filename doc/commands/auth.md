# `cxd auth` — 認証管理

> **日本語** | [English](../en/commands/auth.md)
>
> [← コマンドリファレンス一覧](../commands.md)

API キーや OAuth の認証情報を管理します。

## 独立プロファイル

ホストCLIのログインとは別の認証を使う場合は、プロファイルを明示選択します。選択中はホストの環境変数・認証ファイルへfallbackしません。存在しないプロファイルや権限不備はエラーです。

```bash
cxd auth profile set work --provider openai  # TTYで非表示入力
cxd auth profile set claude-work --provider anthropic
cxd auth profile show work
cxd auth profile list
```

非対話入力には `--key-stdin` を明示します。キーを引数へ書かず、安全な入力元からstdinへ渡してください。保存先は `~/.config/codex-dock/profiles/work/work.json`。rootとプロファイルのディレクトリは `0700`、JSONとlockファイルは `0600`、平文保存です。`--auth-profile-dir` またはグローバル設定の `auth_profile_dir` でrootを変更できます。WSLではLinux側の私有ディレクトリを使い、共有・同期フォルダーを避けてください。

OAuthも、認証元を明示したstdin入力で取り込めます。

```bash
cxd auth profile import work-oauth --format codex --credentials-stdin < /private/product-login/auth.json
cxd auth profile import claude-oauth --format claude --credentials-stdin < /private/product-login/.credentials.json
```

`codex` は公式CLI形式の `tokens` と `last_refresh`、`claude` は `claudeAiOauth` を読みます。既定の `credentials` は `credentialstore.Credentials` のJSON形式で、両providerを含められます。元ファイルは自動探索・削除しません。専用login Kataの自動起動や入力キーの実API照合は未実装です。

管理シークレットも引数に値を載せず、別の `0600` ファイルで渡せます。次の例は既存ファイルを上書きしません。

```bash
python3 - <<'PY'
import os, pathlib, secrets
path = pathlib.Path.home() / '.config/codex-dock/proxy-admin-secret'
with os.fdopen(os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600), 'w') as f:
    f.write(secrets.token_hex(32) + '\n')
PY
cxd --auth-profile work proxy run --admin-secret-file "$HOME/.config/codex-dock/proxy-admin-secret"
cxd --auth-profile work run --agent codex --proxy-admin-secret-file "$HOME/.config/codex-dock/proxy-admin-secret"
```

通常利用では、信頼済みのグローバル設定 `~/.config/codex-dock/config.toml` に選択と管理ファイルのパスを置くと、`init`・`run`・`start`・TUIで同じ設定を使えます。プロジェクト設定には置きません。

```toml
auth_profile = "work"
[proxy]
admin_secret_file = "/home/user/.config/codex-dock/proxy-admin-secret"
```

ネイティブCLI互換の `cxd codex` / `cxd claude` は引数をそのCLIへ渡すため、プロファイル選択にはグローバル設定を使ってください。`cxd run` は上記フラグに対応します。`auth show` は選択したプロファイルのモードだけを表示します。

作成・importは既存プロファイルを上書きしません。再ログインは新しい名前へ取り込み、旧ワーカーを停止・削除した後にProxyを明示的に再作成して切り替えてください。異なるプロファイル・接続世代へ既存ワーカーを再認可しません。OAuthの通常更新は同じプロファイルへCAS保存し、接続世代を維持します。選択した保存ディレクトリだけをAuth Proxyへ読み書き可能でmountし、ワーカーとegressには渡しません。保存先や管理シークレットをworkspace・追加mountへ含めないでください。

以下の従来コマンドはプロファイル未選択時のホスト連携です。選択中の `auth set` / `auth rotate` は拒否します。

---

## `auth show` — 認証状態の確認

```bash
cxd auth show
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
cxd auth set
```

`OPENAI_API_KEY` 環境変数の値を `~/.config/codex-dock/apikey` に保存します。
パーミッションは `0600` で保護されます。

---

## `auth rotate` — トークンのローテーション

```bash
cxd auth rotate
```

現行実装はローカルに `.rotate` マーカーを作成するだけで、稼働中プロキシのトークンを即時失効させません。
即時失効にはワーカー停止または `/admin/revoke` を使い、プロキシ全体の更新は[運用手順](../operations.md#cliプロキシを更新する)を参照してください。

---

## 認証ファイルの場所

| ファイル | 場所 | 説明 |
|---|---|---|
| API キー | `~/.config/codex-dock/apikey` | `cxd auth set` で保存 |
| OAuth クレデンシャル | `~/.codex/auth.json` | Codex CLI が生成（`codex login` で作成） |

詳細は [設定リファレンス — 認証ファイル](../configuration.md#認証ファイルの場所) を参照してください。

---

## 関連ドキュメント

- [Auth Proxy 技術仕様](../auth-proxy.md) — クレデンシャルの優先順位と保護の仕組み
- [設定リファレンス](../configuration.md) — 認証ファイルの形式
- [クイックスタート](../getting-started.md) — 認証の初期設定手順
