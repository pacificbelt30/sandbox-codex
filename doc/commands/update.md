# `oaiso update` — oaiso 自体の更新

> **日本語** | [English](../en/commands/update.md)
>
> [← コマンドリファレンス一覧](../commands.md)

- リリース版の更新：[配布インストーラーを再実行](../setup.md#更新する)します。CLI・node・補助プログラムを同じ版へそろえます。
- このコマンドは `go install` で導入したCLIだけを更新します。

```bash
oaiso update
```

---

## 動作

内部で以下のコマンドを実行し、Go binディレクトリへ `oaiso` を配置します。

```bash
go install github.com/pacificbelt30/codex-dock/cmd/oaiso@latest
```

- `go` コマンドが `PATH` に存在しない場合はエラーになります。
- ソースからのCLI更新用です。通常のリリース導入ではGoを追加せず、配布インストーラーで更新します。
- `oaiso` は `$(go env GOBIN)`、未設定なら `$(go env GOPATH)/bin` に配置されます。
- 旧コマンド・互換aliasは生成しません。既にある旧ファイルの回収は[移行手順](../migration.md#oaisoへの名称変更)を参照。

## 導入方法に合わせて更新する

| 導入方法 | 更新方法 |
|---|---|
| 自動インストール／node bundle | [setupの更新手順](../setup.md#更新する)。CLI・node・helperを同じ版へそろえる |
| 手動配置 | [固定資産とバイナリの更新](../kata.md#固定資産の更新) |
| GoのbinディレクトリのCLIだけ | `oaiso update`。配置済みnode・イメージは別途更新 |

- `command -v oaiso` で実行中の配置先を確認。
- `/usr/local/bin` のCLIは、このコマンドでは更新されない。Goのbinディレクトリへ別のCLIができるだけ。
- `make install` だけでは管理サービスの版がそろわない。管理対象ノードでは上表の更新手順を使用。

---

## 関連ドキュメント

- [インストール](../installation.md) — 新規導入と対応環境

## ホストサービスとイメージの更新

- このコマンドはnode・guard・IO logger・固定資産・プロキシイメージを更新しません。
- [Kataの配置](../kata.md)と[運用時の更新](../operations.md#cliプロキシを更新する)を参照してください。
