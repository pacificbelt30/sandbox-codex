# ネットワーク仕様 (dock-net)

> **日本語** | [English](en/network.md)

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
│            │ host.docker.internal  │                          │
│            │ (host-gateway経由)    │                          │
│            ▼                       ▼                          │
│  ┌──────────────────────────────────────┐                     │
│  │  Auth Proxy (0.0.0.0:PORT)           │                     │
│  │  コンテナから host.docker.internal:PORT で到達可能    │     │
│  └──────────────────────────────────────┘                     │
│            │                                                   │
│            ▼                                                   │
│  ┌──────────────────────────────────────┐                     │
│  │  ホストの外部ネットワーク              │                     │
│  │  (インターネット / OpenAI API 等)     │                     │
│  └──────────────────────────────────────┘                     │
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
| ホストアクセス遮断 | Linux で有効 | `DOCKER-USER` + `iptables` で private/link-local 宛を遮断 |

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

### コンテナ→ホスト通信 (Linux)

Linux では `dock-net` 作成時に `DOCKER-USER` チェーンへ `CODEX-DOCK` チェーンを接続し、
コンテナから private/link-local 宛へ出る通信を遮断します。

| 通信方向 | 状態 | 詳細 |
|---|---|---|
| コンテナ → インターネット | ✅ 許可 | IP Masquerade 経由 |
| コンテナ間 (ICC) | ✅ 遮断 | `enable_icc=false` |
| コンテナ → ホスト/LAN | Linux: ✅ 遮断 | `DOCKER-USER` で `10/8`, `172.16/12`, `192.168/16`, `169.254/16`, `127/8` を DROP |
| ホスト → コンテナ | ✅ 制御可能 | Docker デフォルトポリシー |

> **注意**: この firewall 制御は Linux の `iptables` 前提です。macOS / Windows (Docker Desktop) では自動適用されません。
> Linux では `codex-dock run` / `codex-dock network create` を root で実行する必要があります。

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

各コンテナは `dock-net` に接続されて起動します。また `host.docker.internal` という名前でホストに到達できるよう `ExtraHosts` を設定します：

```go
hostConfig := &container.HostConfig{
    NetworkMode: container.NetworkMode("dock-net"),
    ExtraHosts:  []string{"host.docker.internal:host-gateway"},
    // ...
}
```

`host-gateway` は Docker Engine が自動解決する特殊な値で、コンテナから見たホストのゲートウェイ IP（通常 `docker0` の `172.17.0.1`）に変換されます。

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

> **注意**: Linux では network 作成時に `iptables` ルールも投入するため root 権限が必要です。

### ネットワーク削除

```bash
codex-dock network rm
```

> **注意**: 実行中のコンテナがある場合は先にコンテナを停止してください。

---

## 既知の問題

### ~~F-NET-04~~: Auth Proxy への到達不可 ✅ 解決済み

Auth Proxy は `0.0.0.0:PORT` でリッスンし、コンテナには `--add-host=host.docker.internal:host-gateway` が設定されます。
コンテナは `http://host.docker.internal:PORT` 経由でプロキシに到達します。

```
コンテナ ──▶ host.docker.internal:PORT ──▶ ホスト (172.17.0.1 等) ──▶ Auth Proxy   ✅ 到達可能
```

### F-NET-02: コンテナ→ホスト通信の遮断は Linux 依存

**現状**: Linux では `iptables` により遮断しますが、macOS / Windows では同等の自動制御を未実装です。

**回避策**: Docker Desktop 環境ではホスト OS 側 firewall で同等の egress 制御を追加してください。

---

## 手動で firewall を設定する場合 (Linux)

自動設定と同じ方針を手で入れる場合は、root で以下を実行します。
Auth Proxy をホストの `18080/tcp` で許可する例です。

```bash
sudo iptables -N CODEX-DOCK 2>/dev/null || true
sudo iptables -C DOCKER-USER -i dock-net0 -j CODEX-DOCK 2>/dev/null || \
  sudo iptables -I DOCKER-USER 1 -i dock-net0 -j CODEX-DOCK
sudo iptables -F CODEX-DOCK

# Auth Proxy だけ例外許可したい場合
sudo iptables -A CODEX-DOCK -d 172.17.0.1/32 -p tcp --dport 18080 -j RETURN

# ホスト/LAN 向け private/link-local 宛を遮断
sudo iptables -A CODEX-DOCK -d 10.0.0.0/8 -j DROP
sudo iptables -A CODEX-DOCK -d 172.16.0.0/12 -j DROP
sudo iptables -A CODEX-DOCK -d 192.168.0.0/16 -j DROP
sudo iptables -A CODEX-DOCK -d 169.254.0.0/16 -j DROP
sudo iptables -A CODEX-DOCK -d 127.0.0.0/8 -j DROP
sudo iptables -A CODEX-DOCK -j RETURN
```

削除する場合:

```bash
sudo iptables -D DOCKER-USER -i dock-net0 -j CODEX-DOCK 2>/dev/null || true
sudo iptables -F CODEX-DOCK 2>/dev/null || true
sudo iptables -X CODEX-DOCK 2>/dev/null || true
```

> **注意**: `172.17.0.1` は Linux の標準 bridge 例です。Docker Desktop や独自 bridge 構成では Auth Proxy の許可先 IP を実環境に合わせて読み替えてください。

---

## host.docker.internal トラブルシューティング

`host.docker.internal:host-gateway` を使った Auth Proxy 到達経路でよく発生する問題と対処法をまとめます。

### 前提条件

| 項目 | 要件 |
|---|---|
| Docker Engine | **20.10 以上** (`host-gateway` 特殊値が使用可能) |
| OS | Linux / macOS (Docker Desktop) / Windows (Docker Desktop or WSL2) |
| ネットワーク | `docker0` ブリッジインターフェースが存在すること |

Docker Engine のバージョン確認:
```bash
docker version --format '{{.Server.Version}}'
```

---

### 問題 1: コンテナが Auth Proxy に接続できない（Connection refused）

**症状**:
```
[codex-dock] ERROR: Failed to fetch credentials from Auth Proxy at http://host.docker.internal:PORT
curl: (7) Failed to connect to host.docker.internal port PORT after 0 ms: Connection refused
```

**原因と対処**:

#### 原因 A: `host-gateway` が解決できない（Docker Engine < 20.10）

`host-gateway` 特殊値は Docker Engine 20.10 以降でのみサポートされます。

```bash
# バージョン確認
docker version --format '{{.Server.Version}}'
# 20.10 未満の場合はアップグレードが必要
```

#### 原因 B: `docker0` インターフェースが存在しない

`host-gateway` はデフォルトブリッジ（`docker0`）のゲートウェイ IP に解決されます。
`docker0` が無効化されている環境（一部のクラウド VM や最小構成）では解決に失敗します。

```bash
# docker0 の確認
ip addr show docker0

# docker0 がない場合、デーモン設定でゲートウェイIPを明示指定
# /etc/docker/daemon.json
{
  "host-gateway-ip": "192.168.1.1"  # ← ホストのLAN側IPを指定
}
sudo systemctl restart docker
```

#### 原因 C: ホストのファイアウォールがポートをブロック

Auth Proxy が使うポートはランダム（`:0`）で、起動時に決定されます。

```bash
# iptables で Docker ブリッジからのアクセスを許可する例
sudo iptables -I INPUT -i docker0 -j ACCEPT
sudo iptables -I INPUT -i br-+ -j ACCEPT

# 永続化（Ubuntu/Debian の場合）
sudo apt install iptables-persistent
sudo netfilter-persistent save
```

---

### 問題 2: WSL2 環境で接続できない

**症状**: WSL2 上の Docker で codex-dock を実行すると、コンテナが `host.docker.internal` に到達できない。

**背景**:
WSL2 は Hyper-V ベースの仮想マシン上で動作し、ホスト Windows と WSL2 Linux の間にはネットワーク境界があります。
Docker Desktop を使用している場合は透過的に処理されますが、WSL2 内で直接 Docker Engine を動かす場合は注意が必要です。

#### Docker Desktop (Windows) を使用している場合

Docker Desktop は `host.docker.internal` を自動的に設定するため、通常は追加設定不要です。

```bash
# コンテナ内から確認
docker run --rm alpine cat /etc/hosts | grep host.docker.internal
# → 192.168.65.2  host.docker.internal（Docker Desktop が自動追加）
```

#### WSL2 内の Docker Engine を直接使用している場合

WSL2 の `eth0` インターフェースは Windows 側のネットワークとは別で、`docker0` も WSL2 内にのみ存在します。

```bash
# WSL2 内の docker0 IP を確認
ip addr show docker0 | grep 'inet '
# → inet 172.17.0.1/16 scope global docker0

# docker0 から Auth Proxy への接続は問題ないはずだが
# もし失敗する場合は --host-gateway-ip でIPを明示
# /etc/docker/daemon.json (WSL2 内)
{
  "host-gateway-ip": "172.17.0.1"
}
```

#### Windows Firewall がブロックしている場合

WSL2 → ホスト Windows への通信を Windows Firewall がブロックする場合があります。

```powershell
# PowerShell (管理者) でWSL2からのインバウンドを許可
New-NetFirewallRule -DisplayName "WSL2 Docker Inbound" `
  -Direction Inbound -Protocol TCP `
  -LocalPort 1024-65535 `
  -RemoteAddress 172.16.0.0/12 `
  -Action Allow
```

---

### 問題 3: macOS (Docker Desktop) での注意点

macOS では Docker コンテナはネイティブの Linux 環境ではなく、軽量 Linux VM（Lima や HyperKit）上で動作します。

```bash
# コンテナ内から host.docker.internal が解決できるか確認
docker run --rm --add-host=host.docker.internal:host-gateway alpine \
  sh -c 'getent hosts host.docker.internal'
```

**Docker Desktop for Mac** では `host.docker.internal` は自動的に設定されるため、通常は追加設定不要です。
`--add-host=host.docker.internal:host-gateway` が重複設定になっても問題ありません。

---

### 問題 4: `host-gateway` の解決先が期待と異なる

`host-gateway` が解決する IP は Docker デーモンの設定と実行環境によって変わります。

```bash
# 実際にどのIPに解決されるか確認する
docker run --rm --add-host=host.docker.internal:host-gateway alpine \
  cat /etc/hosts
# → 172.17.0.1  host.docker.internal  （docker0 のゲートウェイIP）
```

解決先が想定と異なる場合は `daemon.json` で明示指定できます:

```json
// /etc/docker/daemon.json
{
  "host-gateway-ip": "172.17.0.1"
}
```

```bash
sudo systemctl restart docker
```

---

### 動作確認手順

codex-dock を起動した後、以下のコマンドで Auth Proxy への到達性を確認できます。

#### 1. Auth Proxy のポートを確認

```bash
# codex-dock run に --verbose を付けて起動
codex-dock run --verbose ...
# → Auth Proxy listening on 0.0.0.0:PORT
```

#### 2. コンテナ内から手動で接続テスト

```bash
# worker コンテナのシェルに入る
codex-dock run --shell

# コンテナ内で接続テスト
curl -sf http://host.docker.internal:PORT/health
# → {"active_tokens":1,"status":"ok"}
```

#### 3. host.docker.internal の名前解決を確認

```bash
# コンテナ内で
getent hosts host.docker.internal
# または
cat /etc/hosts | grep host.docker.internal
```

---

### チェックリスト

接続できない場合は以下を順番に確認してください。

- [ ] `docker version` で Docker Engine >= 20.10 を確認
- [ ] `ip addr show docker0` で docker0 インターフェースが存在することを確認
- [ ] `docker run --rm --add-host=host.docker.internal:host-gateway alpine cat /etc/hosts` で名前解決できることを確認
- [ ] `codex-dock run --verbose` でプロキシのリッスンアドレス（`0.0.0.0:PORT`）を確認
- [ ] ホストのファイアウォール設定で Docker ブリッジからの通信が許可されていることを確認
- [ ] WSL2 の場合は Docker Desktop 経由か直接インストールかを確認
- [ ] macOS の場合は Docker Desktop が起動していることを確認
