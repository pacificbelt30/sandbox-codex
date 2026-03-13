# `codex-dock build` — サンドボックスイメージのビルド

> **日本語** | [English](../../en/commands/build.md)
>
> [← コマンドリファレンス一覧](../commands.md)

サンドボックス用の Docker イメージをビルドします。

```bash
codex-dock build [OPTIONS]
```

---

## オプション

| オプション | 省略形 | デフォルト | 説明 |
|---|---|---|---|
| `--tag` | `-t` | `codex-dock:latest` | イメージのタグ |
| `--dockerfile` | `-f` | （自動検出） | Dockerfile のパス |

---

## Dockerfile の検索順序

`-f` を省略した場合、以下の順序で Dockerfile を自動検出します：

1. カレントディレクトリの `Dockerfile`
2. カレントディレクトリの `docker/Dockerfile`
3. `~/.config/codex-dock/Dockerfile`（存在しない場合は組み込みデフォルトを自動書き出し）

> **補足**: `~/.config/codex-dock/` へのフォールバック時は `entrypoint.sh` も同ディレクトリに書き出されます。
> ユーザーが同ファイルを編集済みの場合は上書きされません。

---

## 使用例

```bash
# デフォルト設定でビルド（Dockerfile を自動検出）
codex-dock build

# カスタムタグでビルド
codex-dock build --tag my-codex:v1

# Dockerfile を明示指定
codex-dock build -f /path/to/Dockerfile

# カスタムイメージをビルドして run で使用
codex-dock build -f ./custom/Dockerfile --tag my-codex:v2
codex-dock run --image my-codex:v2
```

> **省略可能**: `codex-dock run` 実行時にイメージが存在しない場合は自動的にビルドされます。

---

## 関連ドキュメント

- [`codex-dock run`](run.md) — ビルドしたイメージを使ってコンテナを起動
- [クイックスタート](../getting-started.md) — 初回セットアップの手順
