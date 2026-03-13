# Security Design

> [日本語](../security.md) | **English**

Security principles, container configuration, and known limitations of codex-dock.

---

## Core Principle: Never Pass Secrets to Containers

codex-dock's security is built on the principle of **never passing real credentials directly to containers**.

```
                    BAD (conventional approach)
┌────────────┐                           ┌──────────────────┐
│   Host     │  OPENAI_API_KEY=sk-xxx   │    Container     │
│            │─────────────────────────▶│  (risk: leakage) │
└────────────┘                           └──────────────────┘

                    GOOD (codex-dock approach)
┌────────────────────────────────────────────────────────────┐
│   Host                                                      │
│                                                             │
│  API Key ──▶ Auth Proxy ──▶ Placeholder ──▶ Container     │
│  (protected)               (cdx-xxxx, TTL-scoped)          │
│               │                                             │
│               │ Injects real Authorization header           │
│               │ on every outbound request                   │
│               ▼                                             │
│          api.openai.com / chatgpt.com                       │
│                                                             │
│  · Real API Key / access_token never reaches container      │
│  · Container only holds a placeholder (cdx-xxxx)           │
│  · Placeholder cannot be used to access OpenAI directly     │
│  · OAuth refresh_token is held by host only                 │
└────────────────────────────────────────────────────────────┘
```

---

## Container Security Settings

Each sandbox container has the following security settings applied:

| Setting | Value | Effect |
|---|---|---|
| `--cap-drop ALL` | Drop all Linux capabilities | Prevents privilege escalation and privileged operations |
| `--security-opt no-new-privileges` | Prohibit new privilege acquisition | Prevents setuid/setgid binary abuse |
| `USER codex (uid:1000)` | Run as non-root user | Prevents root-level host operations |
| `--pids-limit 512` | Limit maximum processes to 512 | Prevents fork bombs and similar attacks |
| Network: `dock-net` | Bridge network with ICC disabled | Blocks inter-container communication |

---

## Defense-in-Depth Structure

```
[Layer 1: Credential Protection]
  API Key / OAuth tokens → held by Auth Proxy
  Containers receive only short-lived placeholders
        │
        ▼
[Layer 2: Process Privilege Restriction]
  --cap-drop ALL + no-new-privileges + non-root execution
  Malicious behavior inside container cannot easily affect host
        │
        ▼
[Layer 3: Network Isolation]
  dock-net: ICC disabled, IP Masquerade allows internet only
  Prevents lateral movement between containers
        │
        ▼
[Layer 4: Resource Limits]
  --pids-limit 512 prevents fork bombs and similar attacks
```

---

## Implemented Protections

| Protection | Status | Details |
|---|---|---|
| API key isolation | ✅ | Container receives only placeholder; proxy injects real key |
| access_token isolation | ✅ | Real access token never reaches container even in OAuth mode |
| refresh_token protection | ✅ | Never sent to container; proxy handles refresh |
| Short-lived tokens | ✅ | TTL-scoped; immediately revoked on container stop |
| API traffic relay | ✅ | Reverse proxy eliminates direct external API access |
| No credential logging | ✅ | Auth info never written to stdout/stderr |
| Inter-container blocking | ✅ | ICC disabled (`enable_icc=false`) |
| Privilege escalation prevention | ✅ | `--cap-drop ALL` + `--security-opt no-new-privileges` |
| Non-root execution | ✅ | `USER codex (uid:1000)` |
| Resource limits | ✅ | `--pids-limit 512` |

---

## Known Issues

| ID | Issue | Severity | Details |
|---|---|---|---|
| NF-SEC-01 | Auth Proxy uses plaintext HTTP | High | TLS/UNIX socket not implemented; designed for Docker internal use only |
| F-NET-02 | Container-to-host blocking is Linux-specific | Medium | Linux blocks private/link-local egress with `DOCKER-USER` + `iptables` and requires root; macOS / Windows automation is not implemented |
| F-AUTH-06 | No container ID verification | Medium | Token tied to container name but not container ID |

---

## Implementation Status Summary

| Category | Implemented | Partial | Not Implemented |
|---|---|---|---|
| Auth (AUTH) | F-AUTH-01–05, 07 | F-AUTH-06 | F-AUTH-08 |
| Network (NET) | F-NET-01, 03, 04, 05, 06 | F-NET-02 | — |
| Packages (PKG) | F-PKG-01–04, 06 | F-PKG-05 | — |
| Worktree (WT) | F-WT-01–04 | — | F-WT-05 |
| UI | F-UI-01 | F-UI-02, 03 | F-UI-04, 05 |
| Security (SEC) | NF-SEC-02, 03, 04 | NF-SEC-05 | NF-SEC-01, 06 |

---

## Related Documentation

- [Auth Proxy Specification](auth-proxy.md) — Credential protection implementation details
- [Token Lifecycle & Security](auth-proxy/tokens.md) — Security considerations
- [Network Specification](network.md) — dock-net security policy
- [Architecture Overview](architecture.md) — Overall system design
