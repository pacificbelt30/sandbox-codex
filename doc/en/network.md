# Network Specification (dock-net)

> [日本語](../network.md) | **English**

codex-dock uses a dedicated Docker bridge network **dock-net** to isolate containers.

---

## dock-net Configuration

```
┌──────────────────────────────────────────────────────────────┐
│  Host Environment                                              │
│                                                                │
│  ┌──────────────────────────────────────────────────────┐    │
│  │  dock-net  (Docker bridge network)                    │    │
│  │  Subnet: 10.200.0.0/24                               │    │
│  │  Bridge device: dock-net0                             │    │
│  │                                                       │    │
│  │  ┌─────────────┐   ✗ICC   ┌─────────────┐            │    │
│  │  │ Container A │◀────────▶│ Container B │            │    │
│  │  │ (worker-1)  │ blocked  │ (worker-2)  │            │    │
│  │  └──────┬──────┘          └──────┬──────┘            │    │
│  └─────────┼───────────────────────┼────────────────────┘    │
│            │ host.docker.internal  │                          │
│            │ (via host-gateway)    │                          │
│            ▼                       ▼                          │
│  ┌──────────────────────────────────────┐                     │
│  │  Auth Proxy (0.0.0.0:PORT)           │                     │
│  │  Reachable via host.docker.internal:PORT from containers   │
│  └──────────────────────────────────────┘                     │
│            │                                                   │
│            ▼                                                   │
│  ┌──────────────────────────────────────┐                     │
│  │  Host External Network               │                     │
│  │  (Internet / OpenAI API, etc.)       │                     │
│  └──────────────────────────────────────┘                     │
│                                                                │
└──────────────────────────────────────────────────────────────┘
```

---

## Network Configuration Parameters

| Parameter | Value | Description |
|---|---|---|
| Network name | `dock-net` | Docker network name |
| Driver | `bridge` | Docker bridge driver |
| Bridge device name | `dock-net0` | Network interface name on host |
| Subnet | `10.200.0.0/24` | Address space for containers |
| ICC | Disabled (`false`) | Blocks inter-container communication |
| IP Masquerade | Enabled (`true`) | NAT from containers to internet |
| Host access blocking | ⚠️ Partial | iptables rules not yet implemented |

---

## Docker Network Options (Internal)

```go
options := map[string]string{
    "com.docker.network.bridge.enable_icc":           "false",  // ← block inter-container comm
    "com.docker.network.bridge.enable_ip_masquerade": "true",   // ← allow internet access
    "com.docker.network.bridge.name":                 "dock-net0",
}
```

---

## Security Policy

### Inter-Container Communication (ICC) Disabled

Disabling ICC prevents containers on the same `dock-net` from communicating with each other.

```
Container A ──✗──▶ Container B   (no communication within dock-net)
```

**Effect**: Even when multiple parallel workers are running, if one container is compromised it cannot affect others.

### Internet Access

By default, IP Masquerade (NAT) is enabled, allowing containers to access the OpenAI API and other services.

```
Container ──▶ NAT ──▶ Internet (OpenAI API, etc.)   ✅ default
```

Can be disabled with the `--no-internet` flag:

```
Container ──✗──▶ Internet   (when --no-internet is specified)
```

### Container → Host Communication (Partial Implementation)

**Current implementation status (F-NET-02)**: iptables rules to block container-to-host communication are not yet implemented.

| Direction | Status | Details |
|---|---|---|
| Container → Internet | ✅ Allowed | Via IP Masquerade |
| Inter-container (ICC) | ✅ Blocked | `enable_icc=false` |
| Container → Host | ⚠️ Not blocked | iptables rules not implemented |
| Host → Container | ✅ Controllable | Docker default policy |

---

## `--no-internet` Flag

Disables internet access from containers.

```bash
codex-dock run --no-internet
```

Internally sets IP Masquerade to `false`:

```go
if noInternet {
    options["com.docker.network.bridge.enable_ip_masquerade"] = "false"
}
```

> **Note**: With `--no-internet`, containers cannot connect to the OpenAI API either.
> This setting is suitable for code review or read-only tasks.

---

## Container Network Configuration

Each container is started connected to `dock-net`, and `ExtraHosts` is configured so containers can reach the host via `host.docker.internal`:

```go
hostConfig := &container.HostConfig{
    NetworkMode: container.NetworkMode("dock-net"),
    ExtraHosts:  []string{"host.docker.internal:host-gateway"},
    // ...
}
```

`host-gateway` is a special value automatically resolved by Docker Engine to the host gateway IP as seen from containers (typically `docker0`'s `172.17.0.1`).

---

## Network Management Commands

### Check Network Status

```bash
codex-dock network status
```

**Example output:**
```
dock-net status:
  ID:           a1b2c3d4e5f6...
  Driver:       bridge
  Subnet:       10.200.0.0/24
  ICC:          disabled
  IP Masquerade: enabled
```

### Create Network

```bash
codex-dock network create
```

Also created automatically by `codex-dock run`.

### Delete Network

```bash
codex-dock network rm
```

> **Note**: Stop any running containers before deleting the network.

---

## Known Issues

### ~~F-NET-04~~: Auth Proxy Unreachable ✅ Resolved

Auth Proxy listens on `0.0.0.0:PORT`, and containers have `--add-host=host.docker.internal:host-gateway`.
Containers reach the proxy via `http://host.docker.internal:PORT`.

```
Container ──▶ host.docker.internal:PORT ──▶ Host (172.17.0.1, etc.) ──▶ Auth Proxy   ✅ reachable
```

### F-NET-02: Incomplete Container → Host Communication Blocking

**Problem**: `enable_icc=false` blocks inter-container communication but does not block container-to-host communication.

**Workaround (not implemented)**: Add iptables rules using `coreos/go-iptables` or similar.

---

## host.docker.internal Troubleshooting

Common issues and solutions for the Auth Proxy reachability path using `host.docker.internal:host-gateway`.

### Prerequisites

| Item | Requirement |
|---|---|
| Docker Engine | **20.10 or later** (`host-gateway` special value available) |
| OS | Linux / macOS (Docker Desktop) / Windows (Docker Desktop or WSL2) |
| Network | `docker0` bridge interface must exist |

Check Docker Engine version:
```bash
docker version --format '{{.Server.Version}}'
```

---

### Issue 1: Container Cannot Connect to Auth Proxy (Connection refused)

**Symptom**:
```
[codex-dock] ERROR: Failed to fetch credentials from Auth Proxy at http://host.docker.internal:PORT
curl: (7) Failed to connect to host.docker.internal port PORT after 0 ms: Connection refused
```

**Cause and Resolution**:

#### Cause A: `host-gateway` Cannot Be Resolved (Docker Engine < 20.10)

The `host-gateway` special value is only supported on Docker Engine 20.10 and later.

```bash
# Check version
docker version --format '{{.Server.Version}}'
# Upgrade required if below 20.10
```

#### Cause B: `docker0` Interface Does Not Exist

`host-gateway` resolves to the default bridge (`docker0`) gateway IP.
Resolution fails in environments where `docker0` is disabled (some cloud VMs or minimal configurations).

```bash
# Check docker0
ip addr show docker0

# If docker0 is absent, specify gateway IP explicitly in daemon config
# /etc/docker/daemon.json
{
  "host-gateway-ip": "192.168.1.1"  # ← specify host LAN IP
}
sudo systemctl restart docker
```

#### Cause C: Host Firewall Blocking Port

Auth Proxy uses a random port (`:0`), determined at startup.

```bash
# Allow access from Docker bridge via iptables
sudo iptables -I INPUT -i docker0 -j ACCEPT
sudo iptables -I INPUT -i br-+ -j ACCEPT

# Persist (Ubuntu/Debian)
sudo apt install iptables-persistent
sudo netfilter-persistent save
```

---

### Issue 2: Cannot Connect in WSL2 Environment

**Symptom**: When running codex-dock on Docker in WSL2, containers cannot reach `host.docker.internal`.

**Background**:
WSL2 runs on a Hyper-V-based VM, with a network boundary between host Windows and WSL2 Linux.
Docker Desktop handles this transparently, but running Docker Engine directly inside WSL2 requires extra care.

#### When Using Docker Desktop (Windows)

Docker Desktop automatically configures `host.docker.internal`, so no additional setup is usually needed.

```bash
# Verify from inside container
docker run --rm alpine cat /etc/hosts | grep host.docker.internal
# → 192.168.65.2  host.docker.internal (added automatically by Docker Desktop)
```

#### When Using Docker Engine Directly Inside WSL2

WSL2's `eth0` interface is separate from the Windows network, and `docker0` only exists inside WSL2.

```bash
# Check docker0 IP in WSL2
ip addr show docker0 | grep 'inet '
# → inet 172.17.0.1/16 scope global docker0

# Connection from docker0 to Auth Proxy should work
# If it fails, specify IP explicitly with --host-gateway-ip
# /etc/docker/daemon.json (inside WSL2)
{
  "host-gateway-ip": "172.17.0.1"
}
```

#### When Windows Firewall is Blocking

Windows Firewall may block WSL2 → host Windows communication.

```powershell
# Allow WSL2 inbound in PowerShell (Administrator)
New-NetFirewallRule -DisplayName "WSL2 Docker Inbound" `
  -Direction Inbound -Protocol TCP `
  -LocalPort 1024-65535 `
  -RemoteAddress 172.16.0.0/12 `
  -Action Allow
```

---

### Issue 3: Notes for macOS (Docker Desktop)

On macOS, Docker containers run on a lightweight Linux VM (Lima or HyperKit), not natively.

```bash
# Check if host.docker.internal resolves from inside container
docker run --rm --add-host=host.docker.internal:host-gateway alpine \
  sh -c 'getent hosts host.docker.internal'
```

**Docker Desktop for Mac** automatically configures `host.docker.internal`, so no additional setup is usually needed.
Duplicate `--add-host=host.docker.internal:host-gateway` specification is harmless.

---

### Issue 4: `host-gateway` Resolves to Unexpected Address

The IP that `host-gateway` resolves to varies by Docker daemon configuration and environment.

```bash
# Check actual resolved IP
docker run --rm --add-host=host.docker.internal:host-gateway alpine \
  cat /etc/hosts
# → 172.17.0.1  host.docker.internal  (docker0 gateway IP)
```

If the resolved address is not as expected, you can specify it explicitly in `daemon.json`:

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

### Verification Steps

After launching codex-dock, use the following commands to verify Auth Proxy reachability.

#### 1. Check Auth Proxy Port

```bash
# Launch codex-dock run with --verbose
codex-dock run --verbose ...
# → Auth Proxy listening on 0.0.0.0:PORT
```

#### 2. Manual Connection Test from Inside Container

```bash
# Enter worker container shell
codex-dock run --shell

# Connection test inside container
curl -sf http://host.docker.internal:PORT/health
# → {"active_tokens":1,"status":"ok"}
```

#### 3. Verify host.docker.internal Name Resolution

```bash
# Inside container
getent hosts host.docker.internal
# or
cat /etc/hosts | grep host.docker.internal
```

---

### Checklist

If you cannot connect, check the following in order:

- [ ] `docker version` confirms Docker Engine >= 20.10
- [ ] `ip addr show docker0` confirms docker0 interface exists
- [ ] `docker run --rm --add-host=host.docker.internal:host-gateway alpine cat /etc/hosts` confirms name resolution works
- [ ] `codex-dock run --verbose` shows proxy listen address (`0.0.0.0:PORT`)
- [ ] Host firewall allows traffic from Docker bridge
- [ ] For WSL2: verify whether using Docker Desktop or direct install
- [ ] For macOS: verify Docker Desktop is running
