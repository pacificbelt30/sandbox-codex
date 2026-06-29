# 設定リファレンス

> **日本語** | [English](en/configuration.md)

codex-dock の設定は `~/.config/codex-dock/config.toml` に記述します。
TOML 形式で、コマンドラインフラグのデフォルト値を変更できます。

設定ファイルがまだない場合は、最初に次を実行してください。

```bash
make install-config
```

---

## 設定ファイルの場所

| 場所 | 説明 |
|---|---|
| `~/.config/codex-dock/config.toml` | デフォルトの設定ファイル |
| `--config <パス>` | カスタムパスを指定 |

---

## 優先順位

設定値は以下の優先順位で解決されます（上位が優先）：

```
1. コマンドラインフラグ  (最優先)
       │
       ▼
2. 環境変数 (CODEX_DOCK_*)
       │
       ▼
3. config.toml
       │
       ▼
4. ビルトインデフォルト  (最後)
```

---

## 設定項目一覧

> すべての設定キーは [`configs/config.toml.example`](../configs/config.toml.example) にまとまっています。

### `default_image`

サンドボックスコンテナに使用する Docker イメージのデフォルト値。

```toml
default_image = "codex-dock:latest"
```

| 項目 | 内容 |
|---|---|
| 型 | 文字列 |
| デフォルト | `"codex-dock:latest"` |
| 対応フラグ | `--image`, `-i` |
| 環境変数 | `CODEX_DOCK_DEFAULT_IMAGE` |

**例：**
```toml
default_image = "my-custom-sandbox:v2"
```

---

### `default_token_ttl`

Auth Proxy が発行する短命トークンの有効期限（秒単位）。

```toml
default_token_ttl = 3600
```

| 項目 | 内容 |
|---|---|
| 型 | 整数 |
| デフォルト | `3600`（1 時間） |
| 対応フラグ | `--token-ttl` |
| 環境変数 | `CODEX_DOCK_DEFAULT_TOKEN_TTL` |

**例：**
```toml
# 30 分に短縮（よりセキュア）
default_token_ttl = 1800

# 8 時間に延長（長時間タスク用）
default_token_ttl = 28800
```

> **セキュリティ**: TTL を短くするほど、トークンが漏洩した際のリスクが下がります。
> 通常の作業では 1〜2 時間程度が推奨されます。

---

### `proxy_image`

Auth Proxy コンテナに使用する Docker イメージのデフォルト値。

```toml
proxy_image = "codex-dock-proxy:latest"
```

| 項目 | 内容 |
|---|---|
| 型 | 文字列 |
| デフォルト | `"codex-dock-proxy:latest"` |
| 対応箇所 | `proxy build`, `proxy run` |
| 環境変数 | `CODEX_DOCK_PROXY_IMAGE` |

---

### `default_template`

サンドボックスイメージのデフォルトテンプレート。`build` と `run` の両方に適用されます。

```toml
default_template = "plain"
```

| 項目 | 内容 |
|---|---|
| 型 | 文字列 |
| デフォルト | 未設定（テンプレートなし） |
| 対応フラグ | `build --template`, `run --image`（間接） |
| 許可値 | `"plain"`, `"pwn"` など（`codex-dock build --template list` で一覧） |

テンプレートを指定すると、対応するタグのイメージが使用されます（例: `"pwn"` → `codex-dock:pwn`）。
セクション固有の設定（`build.template`、`run.template`）がある場合はそちらが優先されます。

---

### `run.image`

`codex-dock run --image` のデフォルト値を指定します。

```toml
[run]
image = "codex-dock:latest"
```

| 項目 | 内容 |
|---|---|
| 型 | 文字列 |
| デフォルト | 未設定（`default_image` を参照） |
| 対応フラグ | `run --image`, `-i` |

> `run.image` を指定すると、`default_image` より優先されます。

---

### `run.token_ttl`

`codex-dock run --token-ttl` のデフォルト値を指定します。

```toml
[run]
token_ttl = 3600
```

| 項目 | 内容 |
|---|---|
| 型 | 整数 |
| デフォルト | 未設定（`default_token_ttl` を参照） |
| 対応フラグ | `run --token-ttl` |

> `run.token_ttl` を指定すると、`default_token_ttl` より優先されます。

---


### `run.template`

`codex-dock run` で使用するテンプレートのデフォルト値。

```toml
[run]
template = "pwn"
```

| 項目 | 内容 |
|---|---|
| 型 | 文字列 |
| デフォルト | 未設定（`default_template` を参照） |
| 対応フラグ | `run --image`（間接：テンプレートのタグに解決される） |

> `run.template` を指定すると、`default_template` より優先されます。
> `--image` フラグを明示的に指定した場合はテンプレート設定より優先されます。

---

### `run.user`

`codex-dock run --user` のデフォルト値を指定します。

```toml
[run]
user = "current"
```

| 項目 | 内容 |
|---|---|
| 型 | 文字列 |
| デフォルト | `"current"` |
| 対応フラグ | `run --user` |
| 推奨値 | `current`, `codex`, `dir`, `uid[:gid]` |

`codex` を指定すると、従来のデフォルト挙動（コンテナ内 `codex` ユーザ `1001:1001`）で実行されます。

---

### `run.approval_mode`

`codex-dock run --approval-mode` のデフォルト値を指定します。

```toml
[run]
approval_mode = "suggest"
```

| 項目 | 内容 |
|---|---|
| 型 | 文字列 |
| デフォルト | `"suggest"` |
| 対応フラグ | `run --approval-mode` |
| 許可値 | `suggest`, `auto-edit`, `full-auto`, `danger` |

---

### `[build]` セクション — `codex-dock build` の既定値

#### `build.template`

`codex-dock build --template` のデフォルト値。

```toml
[build]
template = "pwn"
```

| 項目 | 内容 |
|---|---|
| 型 | 文字列 |
| デフォルト | 未設定（`default_template` を参照） |
| 対応フラグ | `build --template`, `-T` |

> `build.template` を指定すると、`default_template` より優先されます。
> `--template` フラグを明示的に指定した場合は設定ファイルより優先されます。

#### `build.tag`

`codex-dock build --tag` のデフォルト値。

```toml
[build]
tag = "codex-dock:latest"
```

| 項目 | 内容 |
|---|---|
| 型 | 文字列 |
| デフォルト | `"codex-dock:latest"` |
| 対応フラグ | `build --tag`, `-t` |

---

### `[proxy]` セクション — Auth Proxy / ルータの既定値

> 旧 `[firewall]` セクション（`allow_hosts` / `block_hosts` / `sudo`）は廃止されました。
> ネットワーク隔離は Docker のみで完結し、許可/遮断ルールの設定は不要です。

```toml
[proxy]
# ワーカーコンテナから到達する Auth Proxy の URL（Docker DNS 名 + データプレーンポート）
container_url = "http://codex-auth-proxy:18080"
# フォワードプロキシの到達先ドメイン allowlist（サブドメインも許可。未指定なら全許可）
forward_allow_domains = ["github.com", "registry.npmjs.org", "pypi.org"]
```

#### `proxy.container_url`

| 項目 | 内容 |
|---|---|
| 型 | 文字列 |
| デフォルト | `"http://codex-auth-proxy:18080"` |
| 対応フラグ | `run --proxy-container-url` |

ワーカーが Docker DNS 経由で到達する Auth Proxy のデータプレーン URL。
互換のため旧キー `firewall.proxy_container_url` も当面読み込まれます。

#### `proxy.forward_allow_domains`

| 項目 | 内容 |
|---|---|
| 型 | 文字列の配列 |
| デフォルト | 未設定（全ドメイン許可） |
| 対応フラグ | `proxy run --forward-allow-domain`（繰り返し指定可） |

HTTP CONNECT フォワードプロキシの到達先を、指定ドメイン（およびそのサブドメイン）に制限します。
その他の宛先は 403 になります。

> **メモ**: 特定ワーカーの一般 egress を止めたい場合は `codex-dock run --no-internet` を使います
> （`HTTP(S)_PROXY` を注入せず、プロキシの API リバースルートのみ到達可能になります）。

---

### `network_name`

Auth Proxy が接続する egress ネットワーク名。

```toml
network_name = "dock-net-proxy"
```

| 項目 | 内容 |
|---|---|
| 型 | 文字列 |
| デフォルト | `"dock-net"` |
| 環境変数 | `CODEX_DOCK_NETWORK_NAME` |

> 通常は変更不要です。複数の codex-dock 環境を分離したい場合に使用します。

---

### `verbose`

詳細なログをデフォルトで出力するかどうか。

```toml
verbose = false
```

| 項目 | 内容 |
|---|---|
| 型 | ブール |
| デフォルト | `false` |
| 対応フラグ | `--verbose`, `-v` |
| 環境変数 | `CODEX_DOCK_VERBOSE` |

verbose モードで出力される追加情報：
- Auth Proxy のリッスンアドレス
- トークンの発行・失効・期限切れ
- クレデンシャルのコンテナへの提供
- コンテナ作成の詳細

---

### `debug`

デバッグログをデフォルトで出力するかどうか。

```toml
debug = false
```

| 項目 | 内容 |
|---|---|
| 型 | ブール |
| デフォルト | `false` |
| 対応フラグ | `--debug` |
| 環境変数 | `CODEX_DOCK_DEBUG` |

debug モードで出力される追加情報：
- 発行したトークンの詳細（TTL、コンテナ名）

---

## 設定ファイルの例

```toml
# ~/.config/codex-dock/config.toml
# codex-dock configuration file

# 使用するデフォルトイメージ
default_image = "codex-dock:latest"

# トークンの有効期限（秒）: 1 時間
default_token_ttl = 3600

# egress（プロキシ）ネットワーク名
network_name = "dock-net-proxy"

# デフォルトテンプレート（未指定ならテンプレートなし）
# default_template = "plain"

# 詳細ログ（通常は false）
verbose = false

# デバッグログ（開発・トラブルシューティング時のみ）
debug = false

[run]
# run サブコマンドのデフォルトイメージ（未指定なら default_image を使用）
image = "codex-dock:latest"

# run サブコマンドのテンプレート（未指定なら default_template を使用）
# template = "pwn"

# run サブコマンドのトークン TTL（未指定なら default_token_ttl を使用）
token_ttl = 3600

# run サブコマンドのデフォルトユーザ
user = "current"

# run サブコマンドの承認モード
approval_mode = "suggest"

[build]
# build サブコマンドのテンプレート（未指定なら default_template を使用）
# template = "pwn"

# build サブコマンドのデフォルトタグ
# tag = "codex-dock:latest"

[proxy]
# ワーカーから到達する Auth Proxy URL（--proxy-container-url）
container_url = "http://codex-auth-proxy:18080"

# フォワードプロキシの到達先ドメイン allowlist（--forward-allow-domain 相当）
forward_allow_domains = ["github.com", "registry.npmjs.org", "pypi.org"]
```

---

## 認証ファイルの場所

codex-dock が使用する認証関連ファイルの場所：

| ファイル | 場所 | 説明 |
|---|---|---|
| API キー | `~/.config/codex-dock/apikey` | `codex-dock auth set` で保存 |
| OAuth クレデンシャル | `~/.codex/auth.json` | Codex CLI が生成 |
| トークンローテーションマーカー | `~/.config/codex-dock/.rotate` | `auth rotate` で更新 |

### `~/.config/codex-dock/apikey` の形式

```json
{"key": "sk-..."}
```

パーミッション: `0600`（所有者のみ読み書き可能）

### `~/.codex/auth.json` の形式

**API キーモードの場合：**
```json
{
  "OPENAI_API_KEY": "sk-..."
}
```

**OAuth モード（ChatGPT サブスクリプション）の場合：**
```json
{
  "access_token": "eyJhbGci...",
  "refresh_token": "rt-...",
  "expires_at": 1735689600,
  "token_type": "Bearer"
}
```

> `refresh_token` フィールドが存在する場合、自動的に OAuth モードで動作します。

---

## コンテナ内の環境変数

`codex-dock run` がコンテナに注入する環境変数（参考）：

| 変数名 | 内容 | 変更可能 |
|---|---|---|
| `CODEX_SANDBOX` | 常に `"1"` | 不可 |
| `CODEX_AUTH_PROXY_URL` | Auth Proxy の URL | 不可（自動設定） |
| `CODEX_TOKEN` | 短命トークン | 不可（自動発行） |
| `CODEX_TASK` | タスクプロンプト | `--task` で指定 |
| `CODEX_MODEL` | モデル名 | `--model` で指定 |
| `CODEX_APPROVAL_MODE` | 承認モード (`auto-edit` / `full-auto` / `danger`) | `--approval-mode` で指定 |
| `CODEX_INSTALL_SCRIPT` | パッケージインストールスクリプト | `--pkg` で指定 |
| `CODEX_AGENTS_MD` | AGENTS.md のパス | `--agents-md` で指定 |
