# ワーカー管理コマンド — ps / stop / rm / logs

> **日本語** | [English](../../en/commands/worker.md)
>
> [← コマンドリファレンス一覧](../commands.md)

---

## `codex-dock ps` — ワーカー一覧

実行中のコンテナを一覧表示します。

```bash
codex-dock ps [OPTIONS]
```

| オプション | 省略形 | デフォルト | 説明 |
|---|---|---|---|
| `--all` | `-a` | `false` | 停止済みのコンテナも含めて表示 |

**出力例：**

```
NAME                   STATUS    UPTIME    BRANCH         TASK
codex-brave-atlas      running   5m23s     feature-auth   ユニットテスト作成
codex-calm-beacon      running   2m10s     main           (interactive)
```

---

## `codex-dock stop` — コンテナの停止

実行中のコンテナを停止します。

```bash
codex-dock stop [NAME|ID...] [OPTIONS]
```

| オプション | 省略形 | デフォルト | 説明 |
|---|---|---|---|
| `--all` | `-a` | `false` | 実行中の全コンテナを停止 |
| `--timeout` | | `10` | 強制停止までの待機時間（秒） |

**使用例：**

```bash
# 特定のコンテナを停止
codex-dock stop codex-brave-atlas

# 複数のコンテナを停止
codex-dock stop codex-brave-atlas codex-calm-beacon

# 全コンテナを停止
codex-dock stop --all
```

> コンテナ停止時に Auth Proxy の短命トークンも自動的に失効します。
> 詳細は [トークンライフサイクル](../auth-proxy/tokens.md) を参照してください。

---

## `codex-dock rm` — コンテナの削除

停止済みのコンテナを削除します。

```bash
codex-dock rm [NAME|ID...] [OPTIONS]
```

| オプション | 省略形 | デフォルト | 説明 |
|---|---|---|---|
| `--force` | `-f` | `false` | 実行中のコンテナも強制削除 |

**使用例：**

```bash
# 停止済みコンテナを削除
codex-dock rm codex-brave-atlas

# 実行中のコンテナを強制削除
codex-dock rm --force codex-brave-atlas
```

---

## `codex-dock logs` — ログの表示

コンテナのログを表示します。

```bash
codex-dock logs NAME|ID [OPTIONS]
```

| オプション | 省略形 | デフォルト | 説明 |
|---|---|---|---|
| `--tail` | `-n` | `100` | 末尾から表示する行数 |
| `--follow` | `-f` | `false` | ログをリアルタイムで追跡する |

**使用例：**

```bash
# 直近 100 行を表示
codex-dock logs codex-brave-atlas

# リアルタイムでログを追跡
codex-dock logs codex-brave-atlas --follow

# 直近 50 行を表示
codex-dock logs codex-brave-atlas --tail 50
```

---

## 関連ドキュメント

- [TUI ダッシュボード](ui.md) — コンテナの一元管理
- [`codex-dock run`](run.md) — コンテナの起動
- [クイックスタート](../getting-started.md) — 後片付けの手順
