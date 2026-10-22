# firewall機能の廃止

旧 `oaiso firewall` と `--allow-host` / `--block-host` / `--no-firewall` / `--sudo` は廃止しました。

現在はroot管理サービスが専用bridge・namespace・nftablesで隔離します。
ホストの準備には管理者権限が必要です。通常のCLI操作はoperator groupで行います。

| 目的 | 現行の方法 |
|---|---|
| ワーカー間・ホストへの直接通信を遮断 | [専用ネットワーク](network.md)で管理 |
| 一般通信の許可・拒否 | [プロキシポリシー](proxy-policy.md)で設定 |
| 一般通信を無効化 | `run --no-internet`。AI API経路は維持 |

旧環境からの切り替えは[移行手順](migration.md)を参照してください。
