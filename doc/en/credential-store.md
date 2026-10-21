# Case-scoped credentials and run identity

**English** | [日本語](../credential-store.md)

A host product serving multiple users or cases can run a separate auth proxy for each case. Real credentials stay in the product store and host proxy; worker VMs receive only short-lived tokens. The existing CLI proxy remains available.

## Store contract

Implement `pkg/cxd/credentialstore.Store` in the product. `Load(ctx, caseID)` returns credentials, `Version`, and `ConnectionVersion`. `SaveRefresh(ctx, caseID, expectedVersion, provider, next)` must compare versions atomically in the durable store and return a new `Version` only after committing the OAuth refresh. Return `credentialstore.ErrConflict` or `ErrNotFound` after a login replacement, disconnection, or competing refresh. Never log credential arguments.

Keep `ConnectionVersion` stable across ordinary OAuth refreshes and change it when the connection is replaced. Change `Version` after each refresh. The product owns DB encryption, user authorization, and mapping cases to runs.

`credentialstore.Serve(ctx, socketPath, caseID, store)` exposes only that case over a host Unix socket. Serve each case on a separate socket. The parent directory must be owned by the process and private (`0700`); the socket uses `0600` permissions.

## Startup and execution

1. Allocate a unique proxy name, host admin port, and admin secret of at least 32 characters per case.
2. Call `cxd.EnsureCaseProxy(ctx, cxd.CaseProxyOptions{NodeSocket: nodeSocket, Name: "cxd-user-a", CaseID: "user-a", StoreSocket: socketPath, AdminPort: 18181, AdminSecret: secret})`. Build a matching `codex-dock-proxy:latest` image first. Only the case proxy receives the credential socket mount; the VM does not.
3. Pass the returned `AdminURL` and `ContainerURL`, plus the secret, to `cxd.Open` in `Options`. Set `LaunchRequest.Identity` to `identity.Identity{CaseID: "user-a", RunID: "run-1", ConnectionVersion: "v1", Provider: "openai"}`. Include case, run, attempt, and connection generation in `RequestID`; keep it unchanged on retries of that attempt.
4. On cancellation, call `client.RevokeRun(ctx, id)` to stop new requests and active streams, then `Stop` and `Remove` the worker. On disconnection, call `client.RevokeCase(ctx)`, remove all workers, stop new launches for that case, and call `cxd.RemoveCaseProxy(ctx, nodeSocket, proxy.Name, caseID)`. Removal rejects a proxy referenced by a retained worker.

Use separate proxies, admin secrets, credential sockets, and admin ports for case A and case B. `EnsureCaseProxy` verifies an existing container's case digest, image, socket, and admin port. After restart, the proxy reloads credentials from the store. A changed credential or connection version revokes old issuance; a failed OAuth save revokes the case.

`Identity.Provider` is `openai` or `anthropic` and restricts the proxy to that provider's API routes. `CaseProxyOptions.AllowedModels` optionally restricts authenticated AI requests to exact upstream model IDs, checking their JSON body before upstream I/O or credential injection. Empty preserves unrestricted forwarding. See [additional controls](sandbox-features.md#model-authorization-and-staged-startup) for supported routes and limitations. Per-worker general egress allowlists are described in [network policy](../proxy-policy.md); changes require a new worker.

Case proxies are opt-in. The existing CLI flow using host HOME credentials is unchanged. A case proxy does not read HOME credentials or local OAuth state. Keep its admin port on loopback and never pass the admin secret to a worker or project configuration.

OAuth save failures revoke new requests and active stream leases. Storage recovery alone may not restore a refresh token already rotated upstream. The product must reconcile credentials/connection generation, including re-login if needed, and must not silently reauthorize old runs. Restarting the proxy alone does not guarantee credential recovery.

For official Claude Code subscriptions, `AnthropicOAuth` accepts an access-only setup-token record without `RefreshToken`. Expiry then requires host-side credential replacement and reconnection. A login record with a refresh token and known expiry is refreshed and saved with CAS before use. See [Claude Code](claude-code.md).
