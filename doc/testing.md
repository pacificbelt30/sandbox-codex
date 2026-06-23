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
| Anthropic リバースプロキシ（ハンドラ単体） | `internal/authproxy/anthropic_proxy_test.go` | API キーモード（`x-api-key` 注入）・OAuth モード（`Authorization: Bearer` + `anthropic-beta`）・トークンリフレッシュ・`/admin/mode` |
| Anthropic リバースプロキシ（実リスナー結合テスト） | `internal/authproxy/anthropic_integration_test.go` | 実際の HTTP リスナー経由で、ヘッダ変換・プレースホルダ非漏洩・ボディ／パス保全・**SSE ストリーミングの逐次配信**・エラー透過を検証 |
| Codex リバースプロキシ（実リスナー結合テスト） | `internal/authproxy/codex_integration_test.go` | API キーモード（`/v1`→`api.openai.com`）・OAuth モード（`/v1`→`/codex/*`、`ChatGPT-Account-Id` 注入）・SSE ストリーミングを実リスナー経由で検証 |
| Codex リバースプロキシ・OAuth リフレッシュ（ハンドラ単体） | `internal/authproxy/proxy_test.go` | `/v1` 両モード・`/oauth/token` リフレッシュ中継・WebSocket クレデンシャル注入など |
| エージェント別の環境変数生成 | `internal/sandbox/env_test.go` | `--agent codex/claude/shell` ごとに注入される `CODEX_*` / `ANTHROPIC_*` の出し分け |
| プロキシコンテナ起動引数 | `cmd/proxy_test.go` | OpenAI/Anthropic 双方のクレデンシャルバインド（`-e` / `-v`） |
| Dockerfile 解決 | `cmd/build_test.go` | `docker/sandbox/Dockerfile` を含む検索順序 |

リバースプロキシのテストは `httptest` で上流（`api.anthropic.com` 相当）をモックしているため、
ネットワークアクセスや本物のクレデンシャルを必要としません。SSE ストリーミングのテストは、
上流が 1 つ目のイベントを送って待機してから 2 つ目を送る構成にし、クライアントが 1 つ目を
上流完了より前に受信できることを確認します（バッファリングではなく逐次配信されることの検証）。

> **実エンドポイントでの確認（Claude）**: 無効な認証情報でプロキシを起動し、実際の `api.anthropic.com`
> へ転送した結果も確認済みです。API キーモードでは `invalid x-api-key`、OAuth モードでは
> `Invalid bearer token` という**異なる**認証エラーが返ることから、各モードで正しいヘッダ
> （`x-api-key` / `Authorization: Bearer` + `anthropic-beta`）が本物の API に届き、コンテナの
> プレースホルダが差し替えられていることが裏付けられます。

> **実エンドポイントでの確認（Codex）**: 同様に Codex の 3 経路も実エンドポイントへ転送して確認済みです。
> API キーモードは `api.openai.com/v1/models` がマスクした注入キーをエコー（`invalid_api_key`）、
> OAuth モードは `chatgpt.com/backend-api/codex/responses` が ChatGPT 固有の認証エラーを返却、
> `/oauth/token` リフレッシュ中継は `auth.openai.com/oauth/token` が `token_expired` を返す
> （= ホストのリフレッシュトークンが差し込まれて本物のエンドポイントに到達）ことを確認しました。
> 不正な `cdx` トークンはプロキシ側で 401 拒否され、上流へ転送されません。

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
