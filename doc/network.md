# ネットワーク仕様 (dock-net)

codex-dock は専用の Docker ブリッジネットワーク **dock-net** を使用してコンテナを隔離します。

---

## dock-net の構成

```
┌──────────────────────────────────────────────────────────────┐
│  ホスト環境                                                    │
│                                                                │
│  ┌──────────────────────────────────────────────────────┐    │
│  │  dock-net  (Docker ブリッジネットワーク)               │    │
│  │  サブネット: 10.200.0.0/24                          │    │
│  │  ブリッジ名: dock-net0                                 │    │
│  │                                                       │    │
│  │  ┌─────────────┐   ✗ICC   ┌─────────────┐            │    │
│  │  │ コンテナ A  │◀────────▶│ コンテナ B  │            │    │
│  │  │ (worker-1)  │ 通信NG   │ (worker-2)  │            │    │
│  │  └──────┬──────┘          └──────┬──────┘            │    │
│  └─────────┼───────────────────────┼────────────────────┘    │
│            │ IP Masquerade (NAT)    │                          │
│            ▼                       ▼                          │
│  ┌──────────────────────────────────────┐                     │
│  │  ホストの外部ネットワーク              │                     │
│  │  (インターネット / OpenAI API 等)     │                     │
│  └──────────────────────────────────────┘                     │
│                                                                │
│  10.200.0.1 (dock-net ゲートウェイ、Auth Proxy がリッスン)      │
│  └── Auth Proxy (コンテナから到達可能)                          │
│                                                                │
└──────────────────────────────────────────────────────────────┘
```

---

## ネットワーク設定パラメーター

| パラメーター | 値 | 説明 |
|---|---|---|
| ネットワーク名 | `dock-net` | Docker ネットワーク名 |
| ドライバー | `bridge` | Docker ブリッジドライバー |
| ブリッジデバイス名 | `dock-net0` | ホスト側のネットワークインターフェース名 |
| サブネット | `10.200.0.0/24` | コンテナに割り当てるアドレス空間 |
| ICC | 無効 (`false`) | コンテナ間通信を遮断 |
| IP Masquerade | 有効 (`true`) | コンテナからインターネットへの NAT |
| ホストアクセス遮断 | ⚠️ 部分実装 | iptables ルールは未実装 |

---

## Docker ネットワークオプション（内部）

```go
options := map[string]string{
    "com.docker.network.bridge.enable_icc":           "false",  // ← コンテナ間通信ブロック
    "com.docker.network.bridge.enable_ip_masquerade": "true",   // ← インターネット許可
    "com.docker.network.bridge.name":                 "dock-net0",
}
```

---

## セキュリティポリシー

### コンテナ間通信 (ICC) の無効化

ICC (Inter-Container Communication) を無効にすることで、同じ `dock-net` 上のコンテナが互いに通信できなくなります。

```
コンテナ A ──✗──▶ コンテナ B   (同一 dock-net 内でも通信不可)
```

**効果**: 並列ワーカーが複数起動している場合でも、あるコンテナが侵害されても他のコンテナに影響しません。

### インターネットアクセス

デフォルトでは IP Masquerade（NAT）が有効で、コンテナから OpenAI API 等にアクセスできます。

```
コンテナ ──▶ NAT ──▶ インターネット (OpenAI API 等)   ✅ デフォルト
```

`--no-internet` フラグで無効化できます：

```
コンテナ ──✗──▶ インターネット   (--no-internet 指定時)
```

### コンテナ→ホスト通信 (部分実装)

**現在の実装状況 (F-NET-02)**: コンテナからホストへの通信を遮断する iptables ルールは未実装です。

| 通信方向 | 状態 | 詳細 |
|---|---|---|
| コンテナ → インターネット | ✅ 許可 | IP Masquerade 経由 |
| コンテナ間 (ICC) | ✅ 遮断 | `enable_icc=false` |
| コンテナ → ホスト | ⚠️ 未遮断 | iptables ルール未実装 |
| ホスト → コンテナ | ✅ 制御可能 | Docker デフォルトポリシー |

---

## `--no-internet` フラグ

コンテナからのインターネットアクセスを無効にします。

```bash
codex-dock run --no-internet
```

内部では IP Masquerade を `false` に設定します：

```go
if noInternet {
    options["com.docker.network.bridge.enable_ip_masquerade"] = "false"
}
```

> **注意**: `--no-internet` を指定すると、コンテナから OpenAI API にも接続できなくなります。
> この設定はコードレビューや読み取り専用のタスクに適しています。

---

## コンテナのネットワーク設定

各コンテナは `dock-net` に接続されて起動します：

```go
hostConfig := &container.HostConfig{
    NetworkMode: container.NetworkMode("dock-net"),
    // ...
}
```

---

## ネットワーク管理コマンド

### ネットワーク状態確認

```bash
codex-dock network status
```

**出力例：**
```
dock-net status:
  ID:           a1b2c3d4e5f6...
  Driver:       bridge
  Subnet:       10.200.0.0/24
  ICC:          disabled
  IP Masquerade: enabled
```

### ネットワーク作成

```bash
codex-dock network create
```

`codex-dock run` でも自動的に作成されます。

### ネットワーク削除

```bash
codex-dock network rm
```

> **注意**: 実行中のコンテナがある場合は先にコンテナを停止してください。

---

## 既知の問題

### ~~F-NET-04~~: Auth Proxy への到達不可 ✅ 解決済み

Auth Proxy は dock-net ゲートウェイアドレス `10.200.0.1` でリッスンするため、コンテナから正常に到達できます。

```
コンテナ ──────▶ 10.200.0.1:PORT (Auth Proxy)   ✅ 到達可能
```

### F-NET-02: コンテナ→ホスト通信の不完全な遮断

**問題**: `enable_icc=false` はコンテナ間通信を遮断しますが、コンテナからホストへの通信は遮断しません。

**回避策 (未実装)**: `coreos/go-iptables` 等を使用して iptables ルールを追加する。
