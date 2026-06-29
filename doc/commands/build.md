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
| `--template` | `-T` | | テンプレート名（`plain`, `pwn` 等）。`--template list` で一覧表示 |

> **注意**: `--template` と `--dockerfile` (`-f`) は同時に指定できません。

---

## Dockerfile の検索順序

`-f` を省略した場合、以下の順序で Dockerfile を自動検出します：

1. カレントディレクトリの `Dockerfile`
2. カレントディレクトリの `docker/sandbox/Dockerfile`（旧 `docker/Dockerfile` も後方互換で検出）
3. `~/.config/codex-dock/Dockerfile`（存在しない場合は組み込みデフォルトを自動書き出し）

> サンドボックスイメージには Codex CLI と Claude Code の両方が同梱されます。

> **補足**: `~/.config/codex-dock/` へのフォールバック時は `entrypoint.sh` も同ディレクトリに書き出されます。
> ユーザーが同ファイルを編集済みの場合は上書きされません。

---

## テンプレート

`--template` フラグを使うと、用途に応じた専用イメージをビルドできます。
テンプレートの Dockerfile はバイナリに組み込まれているため、ソースコードのチェックアウトは不要です。

### 利用可能なテンプレート

| テンプレート | タグ | 説明 |
|---|---|---|
| `plain` | `codex-dock:latest` | 必要最低限の構成（Codex CLI + Claude Code + 基本ツール）。デフォルトの Dockerfile と同等 |
| `pwn` | `codex-dock:pwn` | CTF / バイナリ解析向け。pwntools, ptrlib, ropper, pwndbg, radare2, gdb, nasm, strace, ltrace, binwalk 等を含む |

`pwn` などの派生テンプレートはベースイメージ (`codex-dock:latest`) を `FROM` として拡張します。
ベースイメージが未ビルドの場合は自動的に先にビルドされます。

### テンプレートのバリデーション

ビルド時に、テンプレートが必要最低限のツールを含んでいるか静的に検証されます：

- `@openai/codex`（Codex CLI）のインストール
- `@anthropic-ai/claude-code`（Claude Code）のインストール
- `git`, `curl` などの基本ツール
- 非 root ユーザーの作成と切り替え
- `entrypoint.sh` のコピー

派生テンプレート（`FROM codex-dock:*`）の場合はベースイメージを信頼し、これらのチェックはスキップされます。

---

## 使用例

```bash
# デフォルト設定でビルド（Dockerfile を自動検出）
codex-dock build

# テンプレートの一覧を表示
codex-dock build --template list

# plain テンプレートでビルド（デフォルトと同等）
codex-dock build --template plain

# pwn テンプレートでビルド（codex-dock:pwn タグ）
codex-dock build --template pwn

# pwn テンプレートをカスタムタグでビルド
codex-dock build --template pwn --tag my-pwn:v1

# カスタムタグでビルド
codex-dock build --tag my-codex:v1

# Dockerfile を明示指定
codex-dock build -f /path/to/Dockerfile

# カスタムイメージをビルドして run で使用
codex-dock build -f ./custom/Dockerfile --tag my-codex:v2
codex-dock run --image my-codex:v2
```

> **省略可能**: `codex-dock run` 実行時にイメージが存在しない場合は自動的にビルドされます。
> テンプレートに対応するタグ（例: `codex-dock:pwn`）の場合は自動的にテンプレートからビルドされます。

---

## 関連ドキュメント

- [`codex-dock run`](run.md) — ビルドしたイメージを使ってコンテナを起動
- [クイックスタート](../getting-started.md) — 初回セットアップの手順
