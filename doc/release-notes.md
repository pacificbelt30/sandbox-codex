# Oaiso v0.0.2

- Ubuntu 26.04 x86_64向けのビルド済みCLI・補助プログラムを配布。
- `install.sh` は同じリリースのアーカイブとSHA-256を取得し、版と各ファイルの整合性を検証してから配置。
- Go・Make・ソース取得は不要。ホストPythonはuv管理の3.12を使用。
- 通常の導入では配置計画を表示。`--apply` でCLI・Kata・専用containerd・BuildKit・管理サービスを配置。
- 既存の更新拒否・自動復旧・ロールバックを維持。

## インストール

```bash
curl -fsSL https://github.com/pacificbelt30/sandbox-oasis/releases/latest/download/install.sh | bash
curl -fsSL https://github.com/pacificbelt30/sandbox-oasis/releases/latest/download/install.sh | bash -s -- --apply
```

- 対応ホスト：Ubuntu 26.04 x86_64、systemd、KVM、cgroup v2、sudoが使える通常ユーザー。
- 配置後はログアウト・再ログインし、`oaiso runtime-check` で確認。
- 認証を準備して `oaiso init`、続けて `oaiso codex` または `oaiso claude`。
- 固定版はリリースの `install.sh` を保存して使用。`--version v0.0.2` でも版を指定できます。

## 配布ファイル

| ファイル | 内容 |
|---|---|
| `oaiso-linux-amd64.tar.gz` | 同じ版のCLI・起動検証／ログ用プログラム・`bundle.json`・ライセンス／依存ライブラリのnotice |
| `install.sh` | 単独で使えるインストーラー |
| `SHA256SUMS` | 上記2ファイルのSHA-256 |

- Kata等の固定配布物と作業イメージは、導入・初期化時に別途準備します。
- 実APIキー・OAuthトークン・プロジェクトは配布物に含みません。
