# age移行時の依存関係検査

**日本語** | [English](../en/development/dependency-security.md)

## 検査条件と結果（2026-10-24）

| 項目 | 条件・結果 |
|---|---|
| ツール | `govulncheck` v1.9.0、Go 1.27.3、Linux amd64、全Goパッケージ |
| 比較元 | age移行前のcommit `1168123`を別checkoutで検査 |
| 比較元のsymbol検出 | 19件 |
| age移行とHTTP/2修正版の適用後 | 15件。比較元にない新規検出は0件 |
| 未解消 | 既存依存の15件。実際の攻撃可能性の精査と互換更新は別作業 |

- **依存検査は合格ではありません。** 上記15件を未解消として記録します。
- 検出数は静的解析とGo脆弱性DBによるものです。このページは実攻撃の成立を確定したセキュリティ監査報告ではありません。
- [govulncheckの説明](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck)。

## 今回適用した更新

| 対象 | 対応 |
|---|---|
| age | v1.3.2を固定。age自体の検出はなし |
| `golang.org/x/net` | age依存をそのまま解決するとv0.57.0になるため、HTTP/2修正版v0.60.0へ更新 |
| 関連依存 | Goのmodule解決に従い、x/crypto・x/sys・x/term・x/text・x/syncも整合する版へ更新 |
| 最低Go版 | 修正版の要件に合わせて1.26.0。開発・CIは1.27.3。配布バイナリ利用者はGo不要 |

- HTTP/2の[GO-2026-6610](https://pkg.go.dev/vuln/GO-2026-6610)は途中の依存解決で新しく検出され、修正版で除去しました。
- 比較元にもあった[GO-2026-6603](https://pkg.go.dev/vuln/GO-2026-6603)、[GO-2026-6611](https://pkg.go.dev/vuln/GO-2026-6611)、[GO-2026-6612](https://pkg.go.dev/vuln/GO-2026-6612)、[GO-2026-6617](https://pkg.go.dev/vuln/GO-2026-6617)も検出されなくなっています。

## 既存依存の残る検出

| Go module | 現在の版 | symbol検出数 | 次に必要なこと |
|---|---|---:|---|
| containerd SDK | v2.1.5 | 8 | SDKから使う経路・daemon専用機能の区別、互換更新の検証 |
| Docker SDK／型定義 | v28.5.2 | 2 | この製品が使うclient／DTOとdaemon専用処理を区別して精査 |
| gRPC | v1.78.0 | 2 | node／containerd接続への到達性と互換更新を確認 |
| OpenTelemetry | v1.40.0 | 1 | 実際に有効な計測経路と更新を確認 |
| moby/sys/user | v0.4.0 | 1 | UID／GID解決への影響と互換更新を確認 |
| SELinuxライブラリ | v1.12.0 | 1 | mount／label処理への到達性と更新を確認 |

- Goのcontainerd SDKの版と、固定配置したcontainerd daemonの版（2.4.2）は別です。SDK検出数だけで配置済みdaemonの状態を判定しません。
- これらの大型SDK更新を暗号化形式の切替へ混在させず、別の差分と実Kata試験で確認します。
- 生の検査記録は開発時の`.kata-verification/age-20261024/`へ保存し、資格情報・実行ログはGitへ追加しません。
