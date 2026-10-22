# ドキュメントの構成と編集

**日本語** | [English](../en/development/documentation.md)

## 章立て

| 章 | 読者が知りたいこと | 主な内容 |
|---|---|---|
| はじめに | 最初に動かすには | インストール・前提・認証・初回起動 |
| 利用ガイド | 目的の作業をするには | エージェント・ファイル・イメージ・認証・通信・隔離 |
| リファレンス | 正確な指定・契約は何か | CLI・設定・Go API・Credential Store・認証API |
| 管理・運用 | 配置後に維持するには | 日常操作・ホスト更新・VM管理・移行・障害対応・性能 |
| 開発 | 仕組みと検証方法は何か | 構成・ネットワーク・上流追従・テスト・今後の対応 |

## ページを書く

1. 冒頭に目的・前提条件・最初の操作を示す。
2. 操作は番号付き手順と実行コマンドで示す。
3. 選択肢・設定・既定値・制限は表にする。
4. 補足は1項目1要点の短い箇条書きにする。
5. 正確なフラグ・契約はリファレンス、詳細な検証記録は開発のページへリンク。

- 実装済み・未実装・検証済み・未検証を区別。
- 資格情報の値・実トークンを例へ載せない。
- 新しい機能ページ：利用ガイドの該当分類と目次の両方へ追加。
- 日英で章の分類をそろえる。英訳がないページは日本語と明記。
- 既存ページのパスと見出しへのリンクを維持。

## 参考にした公式ドキュメント

確認日：2026-10-22。機能・対応環境を転載せず、情報の分類を参考にしています。

| 製品 | 参考にした構成 |
|---|---|
| [E2B](https://docs.e2b.dev/) | 初回手順・利用例・SDKリファレンスを分離 |
| [Docker Sandboxes](https://docs.docker.com/ai/sandboxes/) | 導入・エージェント・ワークフロー・設定・セキュリティ・CLIを分類 |
| [Daytona](https://www.daytona.io/docs/) | 導入・概念・設定・ツール・セキュリティ・API／CLIを分類 |
| [Anthropic Sandbox Runtime](https://github.com/anthropics/sandbox-runtime) | インストール → 基本操作 → 概要 → 仕組みの順に案内 |
| [Claude CodeのSandbox](https://code.claude.com/docs/en/sandboxing) | 導入・設定・仕組み・トラブルシューティング・制限を分離 |

## 確認する

- 前提：Python 3と `venv`。Ubuntuでは `sudo apt-get install -y python3-venv`。
- 実行場所：リポジトリのルート。

```bash
python3 -m venv "$HOME/.cache/oaiso/docs-venv"
"$HOME/.cache/oaiso/docs-venv/bin/python" -m pip install -r requirements-docs.txt
"$HOME/.cache/oaiso/docs-venv/bin/python" -m mkdocs build --strict
```

- 目次のページ漏れ・リンク切れ・見出しリンクを確認。
- コマンド例は実装と照合。構文確認と実機での動作確認を区別。
