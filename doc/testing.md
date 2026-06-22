# テストと動作確認

> **日本語**
>
> [← ドキュメント一覧](index.md)

codex-dock のプロキシ／サンドボックス分離、Codex CLI / Claude Code 両対応に関する
テスト構成と動作確認手順をまとめます。

---

## 1. 自動テスト

CI（`.github/workflows/ci.yml`）と同じコマンドで実行できます。

```bash
# 全パッケージのユニットテスト（race + カバレッジ）
make test
# または
go test -race -coverprofile=coverage.out -covermode=atomic \
  ./cmd/... ./internal/sandbox/... ./internal/authproxy/... \
  ./internal/network/... ./internal/worktree/... ./internal/config/...

go vet ./...
gofmt -l .                 # 出力が空ならフォーマット済み
golangci-lint run --timeout=5m
go mod tidy && git diff --exit-code go.mod go.sum
```

### 主要なテスト

| 対象 | ファイル | 内容 |
|---|---|---|
| Anthropic クレデンシャル読み込み | `internal/authproxy/anthropic_auth_test.go` | `~/.claude/.credentials.json`（OAuth）・`ANTHROPIC_API_KEY`・保存ファイルの読み込みと優先順位 |
| Anthropic リバースプロキシ | `internal/authproxy/anthropic_proxy_test.go` | API キーモード（`x-api-key` 注入）・OAuth モード（`Authorization: Bearer` + `anthropic-beta`）・トークンリフレッシュ・`/admin/mode` |
| エージェント別の環境変数生成 | `internal/sandbox/env_test.go` | `--agent codex/claude/shell` ごとに注入される `CODEX_*` / `ANTHROPIC_*` の出し分け |
| プロキシコンテナ起動引数 | `cmd/proxy_test.go` | OpenAI/Anthropic 双方のクレデンシャルバインド（`-e` / `-v`） |
| Dockerfile 解決 | `cmd/build_test.go` | `docker/sandbox/Dockerfile` を含む検索順序 |

リバースプロキシのテストは `httptest` で上流（`api.anthropic.com` 相当）をモックしているため、
ネットワークアクセスや本物のクレデンシャルを必要としません。

---

## 2. 手動での動作確認

### プロキシのプロバイダ検出

```bash
go build -o codex-dock .

# Anthropic だけ設定して起動
ANTHROPIC_API_KEY=sk-ant-... ./codex-dock proxy serve --listen 127.0.0.1:18080 &

curl -s http://127.0.0.1:18080/health
# {"active_tokens":0,"status":"ok"}

curl -s http://127.0.0.1:18080/admin/mode
# {"anthropic_available":true,"anthropic_oauth_mode":false,"oauth_mode":false}
```

`anthropic_available: true` であれば `codex-dock run --agent claude` が利用可能です。

### サンドボックスイメージのビルド

```bash
make docker        # docker/sandbox/Dockerfile（Codex CLI + Claude Code）
make proxy-docker  # docker/proxy/Dockerfile（Auth Proxy）

# CLI が同梱されていることを確認
docker run --rm --entrypoint sh codex-dock:latest -c 'codex --version && claude --version'
```

### エンドツーエンド（Claude Code）

```bash
codex-dock network create
codex-dock proxy run                       # ANTHROPIC_API_KEY / ~/.claude を自動バインド
codex-dock run --agent claude --task "list the files in this repo"
```

コンテナ内には本物の Anthropic クレデンシャルは渡らず、プロキシが
`/anthropic/*` への送信時に `x-api-key`（API キー）または `Authorization: Bearer`
（OAuth）へ差し替えます。詳細は [Auth Proxy 技術仕様](auth-proxy.md) を参照してください。

---

## 関連ドキュメント

- [Auth Proxy 技術仕様](auth-proxy.md)
- [コマンドリファレンス: run](commands/run.md)
- [アーキテクチャ概要](architecture.md)
