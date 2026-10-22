# クイックスタート

**日本語** | [English](en/getting-started.md)

| 今の状態 | 次の操作 |
|---|---|
| 実行ホストをまだ用意していない | 先に[Installation — インストール](installation.md)を実施 |
| 実行ホストは準備済み | 1〜4で最初のワーカーを起動。5〜6は任意 |
| 既存環境を更新したい | [日常の運用](operations.md) |

## 1. 実行ホストを確認する

- 実行ユーザー：operator groupに属する通常利用者。
- `run` / `proxy run`：`sudo` 不要。

```bash
oaiso runtime-check
```

| 用途 | 必要なもの |
|---|---|
| VM実行 | Linux x86_64・KVM・cgroup v2・専用containerd・Kata runtime-rs・Dragonball |
| イメージ作成 | BuildKit・`buildctl` |
| ソースからのCLI作成 | Go 1.24.7以上・Make |
| 診断エラーの解決 | [トラブルシューティング](troubleshooting.md) |

- このページはCLIと実行ホストの配置後に実施。未配置なら[インストール](installation.md)へ戻る。
- 以下の `init` は製品ソースのルート、エージェント起動は自分の作業プロジェクトで実行。

## 2. 認証方式を選ぶ

**Proxy起動前に、どちらか一方を選択します。**

| 認証方式 | 準備 | 読み込み元 |
|---|---|---|
| 独立プロファイル | [認証ガイドの1〜3](guides/authentication.md)で資格情報を登録し、プロファイルと管理シークレットを設定 | 選択した製品専用Storeのみ。ホスト認証へfallbackしない |
| 従来のホスト連携 | 下表の環境変数またはログインファイルを準備 | ホスト認証を[優先順位](auth-proxy.md#認証ソース)で選択 |

**従来のホスト連携を選んだ場合**

| エージェント | 利用できる認証 |
|---|---|
| Codex | `OPENAI_API_KEY`・保存済みAPIキー・`~/.codex/auth.json` |
| Claude Code | `ANTHROPIC_API_KEY`・`CLAUDE_CODE_OAUTH_TOKEN`・`~/.claude/.credentials.json`・保存済みAPIキー |

- ログインファイル：`oaiso` を実行する利用者のHOMEに配置。
- OpenAI APIキーの保存：`oaiso auth set`。
- 読み取り権限：Proxyは同じ利用者のUID/GIDで読む。
- `0600` のまま利用可。
- ワーカーへ渡すもの：短命トークン。
- 実APIキー・OAuth access/refresh tokenは渡さない。
- Claude固有の設定：[APIキー・サブスクの手順](claude-code.md)。
- 保存済みAnthropic APIキー：既存ファイルの互換読取。`auth set` はOpenAI専用なので、新規保存には独立プロファイルを使用。

### 一般通信が必要なら、初回起動前に許可する

| 用途 | 初期状態・準備 |
|---|---|
| AIへの推論だけ | 固定AI経路を利用。一般通信の許可は不要 |
| Git・npm・pipなど | 一般通信は既定拒否。[通信ガイド](guides/networking.md)で必要な宛先を許可 |
| 独立プロファイルと通信設定を併用 | 同じグローバル `[proxy]` にキーを統合。管理シークレット設定を残す |

## 3. プロキシとイメージを用意する

**通常利用者で、リポジトリのルートから実行します。**

| 登録した資格情報 | 実行するコマンド（どちらか1つ） |
|---|---|
| OpenAI | `oaiso init --agent codex` |
| Anthropic | `oaiso init --agent claude` |

- 資格情報がない側の `init` まで続けて実行しない。
- 終了時の `Ready.` はホスト・イメージ・Proxyの準備完了。実際の推論成功は次の手順で確認。

| `init` が行うこと | 条件・制限 |
|---|---|
| ホスト診断・認証ソースの存在確認 | 認証本文を表示しない。既存設定を書き換えない |
| ワーカーイメージの作成・互換性検査 | 不足時にビルド |
| Proxy起動 | Proxyイメージがない場合はGoソースが必要 |
| 使用エージェントの選択 | `--agent` → 設定した既定値。シェル既定ならCodexを準備 |
| 認証方式が異なる既存Proxy | ワーカー稼働中は強制変更せずエラー |
| 検証対象外 | 実サービスへの認証成功・疎通・推論 |

- ソースが見つからない場合：クローンした製品ソースへ移動して `init` を再実行。
- 準備後：作業するプロジェクトへ移動。
- Codexを既定にする：`oaiso default codex`。
- プロジェクト限定は `--local`。

### 手動で分けて準備する場合

```bash
oaiso build
oaiso proxy build
oaiso proxy run
```

| Proxy | 役割・接続先 |
|---|---|
| `codex-auth-proxy` | AI APIへの認証付き通信 |
| `codex-http-proxy` | Git・npm・pipなどの一般通信。実資格情報なし |
| 管理API | ホストの `127.0.0.1:18081` に公開 |
| ワーカー向けAPI | ホストへ公開しない |

- `proxy run` がネットワークを自動作成。
- 詳細：[認証プロキシ](auth-proxy.md)。
- 既存チームプロジェクトの独自Dockerfileは、[設定を確認](configuration.md#プロジェクト設定)してプロジェクト側で `init`。Proxyイメージがない場合だけ先に製品ソースで `oaiso proxy build`。

## 4. ワーカーを起動する

- 実行場所：作業するプロジェクトのディレクトリ。
- 共有先：VM内の `/workspace`。
- 共有権限・双方向反映：[ファイル共有と所有者](workspace.md)。
- 製品ソースにいる場合は、起動前に自分の作業プロジェクトへ `cd`。
- 下の起動例は、準備した認証に合うものを1つ選択。

```bash
oaiso codex
# Anthropicを準備した場合はこちら
oaiso run --agent claude
```

**シェル・単発コマンド**

- 以下はCodex認証を準備した場合の例。`oaiso run` の既定を変更済みなら、そのエージェントが起動。

```bash
oaiso run
oaiso run -- ls -la
```

| 起動方法 | 動作 |
|---|---|
| `oaiso codex` | Codexを起動 |
| `oaiso run --agent claude` | Claude Codeを起動 |
| `oaiso run` | 設定した既定値。組み込み既定値は認証済みシェル |
| `oaiso run -- COMMAND` | コマンドを1回実行 |
| Kata検証に失敗 | 起動中止。隔離・権限・資源の確認を省略しない |

- 最初のプロンプトで応答を確認。401・502などの場合は[認証・通信の診断](troubleshooting.md)。
- 前面実行を終了すると、ワーカーは既定で削除。停止・再起動を試す場合は次のバックグラウンド例を使用。

### チームで同じ環境を使う場合

1. リポジトリのルートで `oaiso project init`。
2. `codex-dock.toml` をコミット。
3. 独自ツールはプロジェクトのDockerfileを `[build]` で指定。
4. クローン先でホストと認証を準備。
5. `oaiso project check` → `oaiso init`。

[プロジェクト設定例](configuration.md#プロジェクト設定)

## 5. バックグラウンドで実行する

- 例はCodex用。Claudeでは `--agent claude` に置換。
- 次のコマンドは承認確認を省略するため、共有範囲とタスクを確認してから実行。

```bash
oaiso run --agent codex --name add-tests \
  --task "ユニットテストを追加して" --approval-mode full-auto -d

oaiso ps -a
oaiso logs add-tests --follow
```

- ログ追跡のCtrl+C：追跡だけを終了。ワーカーは継続。
- ブランチも分ける場合：[worktreeの条件と保存先](workspace.md#git-worktree)を確認して `--worktree` などを追加。
- `--approval-mode full-auto`：エージェントの承認確認を省略。
- ファイルへの書き込み範囲：[マウントと承認設定](commands/run.md)で確認。

## 6. 停止・再起動・削除する

```bash
oaiso stop add-tests
oaiso start add-tests
oaiso stop add-tests
# 成果を保存してから削除
oaiso rm add-tests
```

| 操作・条件 | 結果 |
|---|---|
| 停止 | 短命トークンを失効。再起動用のワーカーを保持 |
| 再起動 | 同じワーカーID・workspaceで認証を復元 |
| 削除 | 管理対象のネットワーク・worktreeも回収。成果は先に保存 |
| 前面実行の終了 | 既定で削除。保持する場合は `--keep` |
| ワーカー終了後のProxy | 共有サービスとして動作を継続 |

- 再起動はメインプログラムの再実行。中断した処理・メモリ状態の続きから再開する機能ではない。
- `rm --force` は実行中でも削除する操作。通常の終了確認後は `stop` → `rm`。

## 次に読む

| 目的 | 参照先 |
|---|---|
| VM一覧・並列実行・更新・後片付け | [日常の運用](operations.md) |
| Dockerfile・テンプレート | [カスタムイメージ](commands/build.md) |
| 通信の許可・拒否・`--no-internet` | [通信ポリシー](proxy-policy.md) |
| 起動・認証・通信のエラー | [トラブルシューティング](troubleshooting.md) |
| Claudeの認証 | [APIキー・サブスク](claude-code.md) |
