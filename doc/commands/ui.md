# `oaiso ui` — TUI ダッシュボード

> **日本語** | [English](../en/commands/ui.md)
>
> [← コマンドリファレンス一覧](../commands.md)

全ワーカーをリアルタイムで監視・管理するターミナル UI を起動します。

```bash
oaiso ui
```

---

## 画面レイアウト

**配置を示す概略図。枠・幅・名前・時間は表示例。**

```text
┌──────────────────────────────────────────────────────────────┐
│ 御AgentISOlation (Oaiso)  [2 workers running / 3 total]        │
├──────────────────────────────────────────────────────────────┤
│  NAME              BRANCH      STATUS      UPTIME  TASK       │
│  codex-brave-atl   feature-1   ✅ running  5m      テスト作成 │
│▶ codex-calm-bea    main        ✅ running  2m      (interactive)│
│  codex-old-comet   feature-2   ⏹ exited   8m      完了       │
├──────────────────────────────────────────────────────────────┤
│ [↑↓] 選択  [Enter] ログ  [S] 停止  [D] 削除  [R] 再起動        │
│ [A] 全停止  [E] 拒否履歴  [P] Proxy設定  [Q] 終了              │
└──────────────────────────────────────────────────────────────┘
```

---

## キーバインド

| キー | 一覧画面での動作 |
|---|---|
| `↑` / `↓` | ワーカーを選択 |
| `Enter` | 選択したワーカーの直近200行のログを取得 |
| `S` | 選択したワーカーを停止。停止待ち10秒 |
| `D` | 選択したワーカーを強制削除 |
| `A` | 一覧内の実行中ワーカーをすべて停止 |
| `R` | 停止済みワーカーを再起動 |
| `E` | 選択したワーカーのProxy拒否履歴を取得 |
| `P` | Proxy設定画面を開く |
| `Q` | UIを終了。ログ／拒否履歴画面では一覧へ戻る |

- 英字キー：大文字・小文字の両方を利用可。
- 停止・削除・全停止：確認ダイアログなしで実行。必要な成果は先に保存。
- ログと拒否履歴：開いた時点で取得。継続追跡は `oaiso logs WORKER --follow` / `oaiso policy events --worker WORKER --follow`。
- 認証設定：通常のCLIと同じグローバル設定。独立プロファイルと管理シークレットを[先に設定](../guides/authentication.md)。

> **更新間隔**: コンテナ一覧は 2 秒ごとに自動更新されます。

## Proxy設定画面

```bash
oaiso proxy configure
```

- 上のコマンド：ワーカー一覧を開かず、設定だけ編集。ノード停止中でも利用可。
- 設定先：グローバル設定、または `oaiso --config PATH proxy configure` のファイル。
- 編集項目：ドメイン・CIDR・ポートの許可／拒否、空許可時の既定拒否、上流Proxy URL、ホストCAパス。

| ボタン・キー | 動作 |
|---|---|
| Validate | 入力形式を検証 |
| Test destination | 入力したHOST:PORTの名前・ポートを確認。DNS・実通信は試さない |
| Save | 設定を保存。同時編集があれば拒否 |
| Reload | 保存済み設定を読み直す |
| Back / Esc | 一覧へ戻る。単独起動時は終了 |

**Save成功時の表示**

```text
Saved. Recreate the desired proxies/workers explicitly to apply the new settings.
```

- 設定ファイルへの保存完了。次は成果を保存し、対象のProxy・ワーカーを再作成。

- APIキー・実トークン・管理シークレットの値は編集しない。
- 保存だけでは稼働中Proxyへ反映されない。[通信ガイド](../guides/networking.md)の順序で再作成。

---

## 関連ドキュメント

- [ワーカー管理コマンド](worker.md) — ps / stop / rm / logs
- [`oaiso run`](run.md) — 並列ワーカーの起動
