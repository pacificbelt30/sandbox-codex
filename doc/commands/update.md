# `codex-dock update` — codex-dock 自体の更新

> **日本語** | [English](../en/commands/update.md)
>
> [← コマンドリファレンス一覧](../commands.md)

`codex-dock` バイナリを最新版に更新します。

```bash
codex-dock update
```

---

## 動作

内部で以下のコマンドを実行するだけのラッパーです。

```bash
go install github.com/pacificbelt30/codex-dock@latest
```

- `go` コマンドが `PATH` に存在しない場合はエラーになります。
- ビルドされたバイナリは `$(go env GOPATH)/bin`（通常 `~/go/bin`）に配置されます。

> **注意**: このコマンドは `go install` でインストールした場合のみ有効です。
> `make install`（`/usr/local/bin` など）や配布済みバイナリを使っている場合、
> `go install` の出力先とは異なる場所に既存のバイナリがあるため、
> このコマンドを実行しても実際に使っているバイナリは更新されません。
> その場合は `git pull` 後に `make install` を再実行してください。

---

## 関連ドキュメント

- [クイックスタート](../getting-started.md) — `go install` によるインストール手順
