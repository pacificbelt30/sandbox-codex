# 認証方式とプロファイル

**日本語** | [English](../en/guides/authentication.md)

- 前提：[インストール](../installation.md)を完了。
- 設定場所：信頼済みホスト側。WSL／VMをホストにする場合も同様。

## 1. 認証の分離方法を選ぶ

| 方式 | 資格情報の保持先 | 選ぶ場合 |
|---|---|---|
| 独立プロファイル | 製品専用の0700ディレクトリ・0600ファイル | ホストCLIのログインと製品の認証を分ける |
| 従来のホスト連携 | ホストの認証ファイル・認証環境変数 | 既存のホスト認証を利用する |
| 組み込み製品のStore | 組み込み側が提供する保存先 | ケースごとの認証を[Go API](../credential-store.md)で管理する |

- 独立プロファイル選択時：ホスト認証へfallbackしない。
- 独立プロファイルの現在の保存形式：権限制限付き平文。
- プロジェクトの `codex-dock.toml`：資格情報・認証プロファイル設定を置かない。

## 2. エージェントと認証方式を選ぶ

| エージェント | APIキー | サブスク |
|---|---|---|
| Codex CLI | OpenAI APIキーを[登録](../commands/auth.md#apiキーを登録する) | Codex形式のOAuthを[import](../commands/auth.md#oauthを取り込む) |
| Claude Code | Anthropic APIキーを[登録](../commands/auth.md#apiキーを登録する) | Claude形式のOAuthを[import](../commands/auth.md#oauthを取り込む)。setup-tokenは[Claudeの手順](../claude-code.md) |

- `auth profile set`：TTYから非表示入力。引数へ実キーを書かない。
- `auth profile import`：明示したstdinから入力。元ファイルは自動探索・削除しない。
- 従来のホスト連携：[認証ソースの準備](../getting-started.md#2-認証方式を選ぶ)。

## 3. 管理シークレットとプロファイルを設定する

1. [管理シークレットを作成](../commands/auth.md#管理シークレットを作成し起動する)。資格情報とは別の0600ファイルに保存。
2. [グローバル設定でプロファイルを選択](../commands/auth.md#日常操作で使うプロファイルを固定する)。
3. ソースのルートで `oaiso init` または `oaiso init --agent claude`。
4. [エージェントを起動](agents.md)。

## 更新と切り替え

| 操作 | 扱い |
|---|---|
| 通常のOAuth更新 | 同じStoreへCAS保存。接続世代を維持 |
| 再ログイン・別アカウントへの切り替え | 新規プロファイルへ取り込み、[ワーカーとProxyを切り替える](../commands/auth.md#更新切り替え共有の制限) |
| ワーカーへ渡す資格情報 | 短命トークンのみ。実APIキー・access/refresh/ID tokenは返さない |
| 認証専用Kata・自動login・実APIキー検証 | 未実装 |
| 暗号化Vault・鍵backend | 未実装。[設計要件](../credential-store.md) |

- 保持先・権限・制限：[セキュリティ](../security.md)。
- フラグと保存先：[認証コマンドリファレンス](../commands/auth.md)。
