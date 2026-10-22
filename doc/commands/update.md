# `oaiso update` — oaiso 自体の更新

> **日本語** | [English](../en/commands/update.md)
>
> [← コマンドリファレンス一覧](../commands.md)

`go install` で導入した `oaiso` を最新版に更新します。

```bash
oaiso update
```

---

## 動作

内部で以下のコマンドを実行し、同じ Go bin ディレクトリに `oaiso` コマンドを作成・維持します。

```bash
go install github.com/pacificbelt30/codex-dock@latest
```

- `go` コマンドが `PATH` に存在しない場合はエラーになります。
- ビルドされた `codex-dock` バイナリと `oaiso` / `cxd` の互換aliasは `$(go env GOBIN)`、未設定なら `$(go env GOPATH)/bin` に配置されます。
- その場所に別の `oaiso` ファイルがある場合は上書きせずエラーにします。

> **注意**: このコマンドは `go install` でインストールした場合のみ有効です。
> `make install`（`/usr/local/bin` など）や配布済みバイナリを使っている場合、
> `go install` の出力先とは異なる場所に既存のバイナリがあるため、
> このコマンドを実行しても実際に使っているバイナリは更新されません。
> その場合は `git pull` 後に `make install` を再実行してください。

---

## 関連ドキュメント

- [クイックスタート](../getting-started.md) — `go install` によるインストール手順

## ホストサービスとイメージの更新

- このコマンドはnode・guard・IO logger・固定資産・プロキシイメージを更新しません。
- [Kataの配置](../kata.md)と[運用時の更新](../operations.md#cliプロキシを更新する)を参照してください。
