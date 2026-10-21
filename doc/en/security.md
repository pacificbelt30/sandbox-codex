# Security

[日本語](../security.md) | **English**

Kata runtime-rs and Dragonball give each worker a separate guest kernel.
The root node service verifies pinned assets, host processes and resource limits before releasing user code.
Startup fails if checks fail; it does not fall back to runc or QEMU.

## Worker boundary

| Control | Default |
|---|---|
| User | Non-root; caller's UID/GID |
| Capabilities | All sets cleared |
| Guest restrictions | NoNewPrivs=1, Seccomp=2 |
| Guest resources | 1 CPU, 2 GiB memory, 512 PIDs |
| Host VM envelope | 1 CPU, 3 GiB memory, no swap, 512 PIDs |
| Network | No direct internet, host or peer access; selected proxies only |
| Management | No node/containerd/BuildKit sockets or allocation records mounted in workers |

In a verified Kata worker, Codex's inner sandbox is set to `danger-full-access` to avoid nested namespace/seccomp conflicts.
Agent approval settings are separate. Bind-mounted files remain accessible with the permissions you selected.

## Credentials

The auth proxy retains real API keys and OAuth access/refresh tokens.
Workers use short-lived placeholders. OAuth ID tokens and account metadata may be returned for client compatibility.
The egress proxy receives no real credentials.

The auth proxy runs as the caller's UID/GID, with a private HOME and read-only credential binds.
It can read owner-only `0600` files without changing their permissions or adding capabilities.
HTTP and WebSocket AI routes validate issued, unexpired tokens before contacting upstreams.
Revocation, expiry or token replacement cancels active streams and tunnels.
Admin operations are on a separate listener, unreachable from worker networks.

## Trusted host operators

Node socket and BuildKit access are root-equivalent: operators can select commands, images and host mounts.
Runtime assets, tools and configuration are pinned in a root-owned manifest.
The manifest does not replace verification of release provenance or checksums.
Every validation rereads and hashes every asset. Each validation checks files in parallel, limited to GOMAXPROCS and at most four readers. Verification results are never cached based on file timestamps. See the [measurement guide](performance.md).

Recovery verifies all occupants of a dedicated resource group before using `cgroup.kill`.
It does not adopt arbitrary PIDs or other products' VMs.

## Limits

Worker-to-proxy traffic is plain HTTP on dedicated networks; TLS is not provided.
Tokens are bearer credentials, without source container-ID attestation.
General egress rules are immutable per proxy. Worker-specific allowlists select policy-specific proxies; identical policies can share a proxy. Default deny is opt-in; an omitted empty allowlist retains public-destination access. Hot reload is not provided.

`policy quarantine` / Go API `Quarantine` stops one worker and durably prohibits restart at the node. It ends all work, general communication and AI communication, retaining the workspace. Resume under new rules with a new worker. Errors do not establish completed isolation; retry. See [quarantine semantics](sandbox-features.md#stop-communication-and-quarantine).
Explicitly shared files and Git metadata can still be modified where writable.
Isolation also depends on maintaining the host kernel, KVM, Kata and guest kernel.

[Verification](../testing.md) (Japanese) includes real Kata acceptance and existing-auth.json WebSocket/HTTP SSE inference.
Long-running real OAuth refresh, interactive keyboard use and arbitrary custom images require separate checks.

Model ID enforcement is opt-in at the host auth proxy; a model flag alone is not authorization. See [additional controls](sandbox-features.md#model-authorization-and-staged-startup) for supported routes, WebSocket denial and separate egress/broker paths.

Claude Code uses distinct API-key and subscription placeholder variables. No real Claude access/refresh token is written to the worker. Live Anthropic inference and subscription usage accounting remain outside fixture verification; see [Claude Code](claude-code.md) and the [test record](../testing.md#claude-codeの受入) (Japanese).
