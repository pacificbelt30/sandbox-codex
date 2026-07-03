# `codex-dock build` — サンドボックスイメージのビルド

> **日本語** | [English](../en/commands/build.md)
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

### ユーザーテンプレート

`pwn` のように `docker/templates/` 以下に組み込まれたテンプレートとは別に、
**`~/.config/codex-dock/templates/` 以下に自分専用のテンプレートを置くことができます。**
codex-dock のバイナリを再ビルドする必要はなく、Dockerfile を1つ配置するだけで
`--template` から参照できるようになります。私的な用途のテンプレートや、社内限定のテンプレートなど、
リポジトリにコミットしたくないものの置き場所として使えます。

#### 配置場所

```
~/.config/codex-dock/templates/
└── <テンプレート名>/
    └── Dockerfile
```

例えば `mytool` という名前のテンプレートを追加する場合：

```bash
mkdir -p ~/.config/codex-dock/templates/mytool
cp path/to/Dockerfile ~/.config/codex-dock/templates/mytool/Dockerfile

# 動作確認（一覧に [user] 付きで表示される）
codex-dock build --template list
#   Available templates:
#     plain        codex-dock:latest
#     pwn          codex-dock:pwn (extends codex-dock:latest)
#     mytool       codex-dock:mytool [user]
#
#   User templates directory: /home/you/.config/codex-dock/templates
#   Add <dir>/<name>/Dockerfile to add a template or override a built-in one.

# ビルド
codex-dock build --template mytool
```

#### 優先順位（上書き）

`~/.config/codex-dock/templates/<name>/` に組み込みテンプレートと同じ名前（例: `pwn`）の
ディレクトリを置くと、そちらが優先して使われます。手元で組み込みテンプレートの内容を
一時的に試したい・カスタマイズしたい場合に、リポジトリ側のファイルを直接編集する必要はありません。

| 優先度 | 配置場所 |
|---|---|
| 1（優先） | `~/.config/codex-dock/templates/<name>/Dockerfile`（ユーザーテンプレート） |
| 2 | `docker/templates/<name>/Dockerfile`（組み込みテンプレート、バイナリに埋め込み） |

#### バリデーション

ユーザーテンプレートも、次の「テンプレートのバリデーション」節にある組み込みテンプレートと
**まったく同じ静的チェック**（`template.Validate()`）を通ります。`FROM codex-dock:*` で
既存イメージを拡張する派生テンプレートであれば、そのチェックはスキップされます。
そのため、動作確認（verify）や管理のしやすさは組み込みテンプレートと変わりません。

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
