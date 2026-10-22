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

The auth proxy retains real API keys and OAuth access/refresh/ID tokens.
Workers use short-lived access placeholders. The Codex ID field contains a fresh dummy JWT with only account ID, plan and the FedRAMP flag. Original headers, signatures, email, user IDs and arbitrary claims are not forwarded. This dummy JWT is not used to authenticate upstream requests.
The egress proxy receives no real credentials.

Initial acquisition and refresh return only selected worker fields. Refresh errors retain HTTP status and known codes needed for CLI recovery, without forwarding issuer bodies or transport error details. See the [pinned CLI test](../testing.md#codexのidトークン分離) (Japanese). Existing worker files and historical logs are not automatically erased: update the proxy, then stop and recreate existing workers.

The legacy auth proxy runs as the caller's UID/GID, with a private HOME and read-only credential binds. Independent profiles omit host authentication mounts/environment and mount only the selected `0700` directory read-write for CAS/fsync/atomic-rename refresh. Credentials and locks are `0600`. Worker restart verifies the profile and connection generation. Neither mode changes source permissions or adds capabilities. Admin authority can come from a separate `0600` file mounted read-only into auth only, without its value in argv. The CLI rejects profile-storage overlaps with worker shares; embedding callers must also keep credentials and admin authority outside those shares.
Current product API-key files and local OAuth rotation state remain permission-restricted plaintext; the encrypted vault is not implemented. Future [credential-management requirements](credential-store.md#independent-credential-management-wsl-vms-and-headless-systems) include WSL 2, Linux VMs and headless systems. A trusted control environment may itself run in a VM; it is distinct from the untrusted work VM that must never receive real credentials.
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
General egress rules are immutable per proxy. Worker-specific allowlists select policy-specific proxies; identical policies can share a proxy. Default deny is enabled unless explicitly disabled by the host; empty destination grants disconnect general egress. Hot reload is not provided.

`policy quarantine` / Go API `Quarantine` stops one worker and durably prohibits restart at the node. It ends all work, general communication and AI communication, retaining the workspace. Resume under new rules with a new worker. Errors do not establish completed isolation; retry. See [quarantine semantics](sandbox-features.md#stop-communication-and-quarantine).
Explicitly shared files and Git metadata can still be modified where writable.
Isolation also depends on maintaining the host kernel, KVM, Kata and guest kernel.

[Verification](../testing.md) (Japanese) includes real Kata acceptance and existing-auth.json WebSocket/HTTP SSE inference.
Long-running real OAuth refresh, interactive keyboard use and arbitrary custom images require separate checks.

Model ID enforcement is opt-in at the host auth proxy; a model flag alone is not authorization. See [additional controls](sandbox-features.md#model-authorization-and-staged-startup) for supported routes, WebSocket denial and separate egress/broker paths.

Claude Code uses distinct API-key and subscription placeholder variables. No real Claude access/refresh token is written to the worker. Live Anthropic inference and subscription usage accounting remain outside fixture verification; see [Claude Code](claude-code.md) and the [test record](../testing.md#claude-codeの受入) (Japanese).
