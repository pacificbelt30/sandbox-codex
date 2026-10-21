# ケース別の認証と実行ID

[English](en/credential-store.md) | **日本語**

同じホストで複数の利用者やケースを扱う製品は、ケースごとに独立した認証プロキシを使えます。実資格情報は製品の保存先とホスト側プロキシだけに置き、ワーカーVMには短命トークンだけを渡します。既存のCLI用プロキシは引き続き利用できます。

## 製品側の保存契約

製品は `pkg/cxd/credentialstore.Store` を実装します。`Load(ctx, caseID)` は現在の資格情報と `Version`、`ConnectionVersion` を返します。`SaveRefresh(ctx, caseID, expectedVersion, provider, next)` は、保存先で版を原子的に比較し、OAuth更新結果を永続化した後に新しい `Version` を返してください。再ログイン・接続解除・別の更新で版が変わった場合は `credentialstore.ErrConflict` または `ErrNotFound` を返します。資格情報をログへ書かないでください。

`ConnectionVersion` は再ログインなどの接続世代です。OAuthの通常更新では維持し、接続の入れ替え時に変更します。`Version` は更新ごとに変更します。両方を混同すると、古い更新を保存したり、正常な更新後の実行を不要に止めたりします。

`credentialstore.Serve(ctx, socketPath, caseID, store)` が一つのケースだけを扱うホスト内のUnixソケットを提供します。ケースごとに別のソケットを立ててください。親ディレクトリは実行ユーザー所有の `0700`、ソケットは `0600` です。製品DBの暗号化、利用者の認可、ケースIDと実行IDの割り当ては製品が担当します。

## 起動と実行

1. ケースごとに一意のプロキシ名・ホスト管理ポート・管理シークレットを割り当てます。管理シークレットは32文字以上とし、ホストで保持します。
2. `cxd.EnsureCaseProxy(ctx, cxd.CaseProxyOptions{NodeSocket: nodeSocket, Name: "cxd-user-a", CaseID: "user-a", StoreSocket: socketPath, AdminPort: 18181, AdminSecret: secret})` を呼びます。資格情報ソケットは該当プロキシにのみマウントされ、VMには渡されません。互換する `codex-dock-proxy:latest` イメージを事前にビルドしてください。
3. 戻り値の `AdminURL`、`ContainerURL` と同じシークレットを `cxd.Open` の `Options` に指定します。`LaunchRequest.Identity` に `identity.Identity{CaseID: "user-a", RunID: "run-1", ConnectionVersion: "v1", Provider: "openai"}` を入れます。`RequestID` にケース・実行・試行・接続世代を含め、同じ試行の再送では変更しません。
4. 取消時は `client.RevokeRun(ctx, id)` で新規要求と実行中のストリームを止め、`Stop` と `Remove` でVMを回収します。接続解除時は `client.RevokeCase(ctx)` を呼び、全ワーカーを回収します。そのケースで新規起動がない状態で `cxd.RemoveCaseProxy(ctx, nodeSocket, proxy.Name, caseID)` を呼びます。参照するワーカーが残る間は削除を拒否します。

ケースAとケースBは別のプロキシ、別の管理シークレット、別の資格情報ソケット、別の管理ポートを使ってください。`EnsureCaseProxy` は既存コンテナのケースIDのダイジェスト、イメージ、ソケット、管理ポートを照合します。プロキシ再起動時は保存先から資格情報を再取得します。接続世代や資格情報版が変わった場合、旧プロキシは新規トークンを拒否します。OAuth保存失敗時もケースを失効させます。

`Identity.Provider` は `openai` または `anthropic` で、プロキシは該当するAPI経路だけを許可します。`CaseProxyOptions.AllowedModels` に正確な上流モデルIDの許可リストを渡すと、認証済みのAI要求のJSON本文を上流接続・秘密注入前に検査します。空リストは従来通り制限しません。詳細と対応経路は[追加制御](sandbox-features.md#モデル認可と段階的な起動)を参照してください。一般通信のワーカー別ドメイン許可は[通信ポリシー](proxy-policy.md)で設定します。ポリシーを変更する場合は新しいワーカーを起動してください。

ケース別プロキシはオプトインです。既存のホストHOME認証ファイルを使うCLIフローは変わりません。ケース別プロキシではHOME認証ファイルやローカルOAuth状態を読みません。管理ポートにはローカルホストからのみ接続し、管理シークレットをワーカーやプロジェクト設定へ渡さないでください。

OAuth保存失敗時は新規要求と通信中streamを失効させます。保存先が復旧しても、上流でrefresh tokenが既に回転している可能性があります。製品側で再ログインなどの復旧と接続世代の整合性を確認し、古い実行を暗黙に再認可しないでください。proxy再起動だけで資格情報が復旧する保証はありません。

公式Claude Codeのサブスクでは、`AnthropicOAuth` にaccess tokenだけのsetup-tokenを指定でき、`RefreshToken` は省略可能です。その場合、期限切れ時は製品側で資格情報を再発行し再接続します。refresh tokenと既知の期限を持つログインは、更新結果をCAS保存してから使います。[Claude Code](claude-code.md)を参照してください。
