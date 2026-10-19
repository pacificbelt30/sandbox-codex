# cxd ドキュメント

**日本語** | [English](en/index.md)

cxdは、Codex CLIとClaude CodeをKataのVMで隔離して実行します。
containerdがワーカーを管理し、DragonballがVMを動かします。
Docker EngineとDocker CLIは不要です。

## 初めて使う

| 読む順序 | 内容 |
|---|---|
| 1. [実行ホストの準備](kata.md) | 管理者向け。Linux/KVM、Kata、containerd、BuildKitを配置する |
| 2. [クイックスタート](getting-started.md) | 利用者向け。認証・イメージを用意して最初のワーカーを起動する |
| 3. [日常の運用](operations.md) | 一覧、ログ、停止、再起動、更新、後片付け |

旧Docker環境を使っている場合は、[移行手順](migration.md)から始めてください。

## 目的から探す

| 目的 | ページ |
|---|---|
| ファイル共有・所有者・権限を確認する | [workspace](workspace.md) |
| VMの状態や残存資源を調べる | [VM管理](vm.md) |
| カスタムイメージやテンプレートを作る | [build](commands/build.md) |
| 一般通信の許可／拒否を設定する | [通信ポリシー](proxy-policy.md) |
| 認証情報の扱いを知る | [認証プロキシ](auth-proxy.md) |
| プロキシだけを使う | [単体利用](proxy-standalone.md) |
| 起動や通信の問題を調べる | [トラブルシューティング](troubleshooting.md) |
| フラグや設定名を調べる | [コマンド](commands.md)・[設定](configuration.md) |

## 仕組みと開発

- [アーキテクチャ](architecture.md)：各コンポーネントと起動の流れ
- [ネットワーク](network.md)：ワーカーと2種類のプロキシの接続
- [セキュリティ](security.md)：保護する範囲と制限
- [認証API](auth-proxy/endpoints.md)・[短命トークン](auth-proxy/tokens.md)
- [テストと動作確認](testing.md)：自動テスト、実Kata試験、確認済みの範囲

通常の実行先はLinux x86_64です。CLIのクロスビルドは、各OSでKataを実行できることを意味しません。
