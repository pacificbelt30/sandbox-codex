# 一般通信を許可する

**日本語** | [English](../en/guides/networking.md)

| 通信 | 既定の扱い |
|---|---|
| Codex／Claudeの固定AI経路 | 短命トークン・provider・モデル権限で認可 |
| Git・npm・pip・任意のWeb接続 | 一般egress。許可ドメイン・CIDRがなければ接続しない |
| VMからの直接通信・ホスト・他ワーカー宛て | 遮断 |

## 1. ホスト側の許可先を設定

- 設定先：`~/.config/codex-dock/config.toml`。
- 以下はGitHubとnpmの例。実際に必要な配布先・redirect先も確認して追加。
- 既存の `[proxy]` へ統合し、`admin_secret_file` など他のキーを残す。

```toml
[proxy]
forward_default_deny = true
forward_allow_domains = ["github.com", "registry.npmjs.org"]
forward_allow_ports = [80, 443]
```

| 指定 | 意味 |
|---|---|
| `github.com` | その名前とサブドメインを許可 |
| `=api.github.com` | その名前だけに完全一致 |
| ポートだけの許可 | 宛先の許可にはならない |
| 許可ドメインとCIDRの両方を指定 | 両方を満たす必要あり。拒否リストは常に優先 |
| private・loopback・metadata宛て | CIDR許可でも解除できない |

- GitHubのasset配布など、別ドメインへの移動先は上の例に含まれない。
- SSH・JVM・独自CA：[互換設定](../sandbox-features.md)も確認。

## 2. 起動前に確認

```bash
oaiso policy check --json
oaiso policy explain registry.npmjs.org:443 --json
```

- 対象：新規ワーカー向けのホスト設定。稼働中の通信変更ではない。
- `explain`：接続しない。上の例は名前・ポートだけを確認。
- `--resolve` を追加：DNS応答のIPも確認。実サービスへの疎通試験ではない。
- TUIで編集する場合：`oaiso proxy configure`。ノード停止中も利用可。

## 3. 新しいワーカーへ適用

| 今の状態 | 操作 |
|---|---|
| 初回準備中 | 設定後、[クイックスタート](../getting-started.md#3-プロキシとイメージを用意する)の `init` へ進む |
| 既存Proxy・ワーカーあり | 成果保存 → ワーカー停止・削除 → 対象Proxyを停止・削除 → 新設定で `init` → 新規ワーカー |

- 設定の保存だけでは既存Proxy・CONNECT・ワーカーへ反映されない。
- 手順と対象名：[Proxy再作成](../operations.md#cliプロキシを更新する)。
- ワーカー別の `--egress-allow-domain` とプリセット：ホストの許可を狭める指定。ホスト未許可の宛先は追加できない。

## 4. 実行後に確認・停止

```bash
oaiso policy show --worker WORKER --json
oaiso policy events --worker WORKER --json
```

- `WORKER`：`oaiso ps -a` に表示された名前へ置換。
- 拒否履歴：同じegress Proxyを使うワーカー間で共有。実行ごとの独立監査ログではない。
- すぐに遮断する場合：`oaiso policy quarantine --worker WORKER`。**通信と全ジョブを停止し、そのワーカーの再起動を禁止**。
- 隔離後の成果・保持期限・失敗時の扱い：[隔離の仕様](../sandbox-features.md#通信の停止と隔離)。
- 判定規則の詳細：[通信ポリシー](../proxy-policy.md)。
