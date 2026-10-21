# Go API for embedding cxd

[日本語](../go-api.md) | **English**

`github.com/pacificbelt30/codex-dock/pkg/cxd` lets a trusted product on the same host manage workers through the existing `cxd node serve` Unix socket. It does not expose a new network service. Access to the root managed socket permits arbitrary images, commands, and host mounts, so grant it only to trusted host processes.

## Before connecting

1. [Install the execution host](../setup.md) (Japanese) with the node, containerd, Kata, and BuildKit. Give the product access to the node socket at `/run/codex-dock-node.sock`, or set `Options.NodeSocket` to another path.
2. Start the [auth proxy](getting-started.md) and build or import the worker image. `Open` does not start a proxy or create an image.
3. Deploy the product's Go package with a compatible node. `Open` validates the node and its Go client protocol; an incompatible version returns `ErrVersionMismatch`. Update the embedded package from the same source revision when updating the CLI and node.

The default proxy endpoints are `http://127.0.0.1:18081` for the host admin API, `http://codex-auth-proxy:18080` for guest authentication, and `http://codex-http-proxy:18082` for general egress. Set `Options.ProxyAdminURL`, `ProxyContainerURL`, and `HTTPProxyURL` if your deployment differs. Keep `Options.ProxyAdminSecret` in the host product process.

## Example

```go
package main

import (
    "context"
    "errors"
    "os"
    "time"

    "github.com/pacificbelt30/codex-dock/pkg/cxd"
)

func main() {
    ctx := context.Background()
    client, err := cxd.Open(ctx, cxd.Options{
        ProxyAdminSecret: os.Getenv("CODEX_DOCK_PROXY_ADMIN_SECRET"),
    })
    if err != nil { panic(err) }
    defer client.Close()

    worker, err := client.Launch(ctx, cxd.LaunchRequest{
        RequestID:     "product/project-7/job-123/attempt-1",
        ProjectDir:    "/srv/work/project-7",
        Image:         "codex-dock:latest",
        Profile:       "small",
        NoGitMetadata: true,
    })
    if errors.Is(err, cxd.ErrInProgress) {
        // Retry later with the same RequestID.
        return
    }
    if err != nil { panic(err) }
    defer client.Remove(context.Background(), worker.ID, true)

    code, err := client.Exec(ctx, worker.ID, cxd.ExecRequest{
        Argv:   []string{"codex", "exec", "Fix the tests"},
        Stdout: os.Stdout,
        Stderr: os.Stderr,
    })
    if err != nil { panic(err) }
    if code != 0 { panic("codex exited unsuccessfully") }
    if err := client.Stop(ctx, worker.ID, 10*time.Second); err != nil { panic(err) }
}
```

With no `Agent`, `Launch` starts an idle worker for repeated `Exec` calls. `Agent: "codex"` or `"claude"` runs that CLI as the main process. `Exec` returns the guest exit code separately from transport errors. `Wait` waits for the main process, not an individual `Exec` command.

| Method | Contract |
|---|---|
| `Launch(ctx, request)` | Creates and starts a worker under a stable `RequestID`; `ProjectDir` must be absolute. Initial creation and start are one operation |
| `Get` / `List` | Inspect worker state |
| `Exec` / `Wait` | Execute a command in a worker / wait for its main process |
| `Start` / `Stop` / `Remove` | Restart a stopped worker / stop it and revoke its auth token / remove its resources |
| `Request` / `RemoveRequest` | Query durable launch state by request ID / retry cleanup after a lost response |
| `Profiles(ctx)` | Query the running node's CPU, guest RAM, host memory reservation, and PID limits. These are reservation limits, not measured RSS |
| `Capabilities()` / `ProxyCapabilities(ctx)` | Query node build, protocol, and features / connected auth proxy features |

`LaunchRequest` also supports model, approval mode, extra mounts, read-only rootfs, and disabling shared Git metadata. Its default image is `codex-dock:latest` and default resource profile is `standard`. Query `client.Profiles(ctx)` before selecting the smaller `small` or lightweight `compact` (guest 512 MiB / host 768 MiB) profile.

`client.PrepareProject(ctx, dir, requestID, log)` validates the portable `codex-dock.toml`, builds a missing or stale image through BuildKit, and returns a `LaunchRequest` for the product to review and submit. `client.Build` and `client.BuildTemplate` are available separately. `Launch` itself requires a built image and a running auth proxy.

`PrepareProject` does not load global settings or real credentials. It copies project `egress_allow_domains` into `LaunchRequest.EgressAllowDomains`. The Go API starts or reuses a dedicated egress proxy identified by the same policy ID as the CLI. `Options.EgressPolicy` sets host limits that a worker allowlist cannot expand. Build the matching proxy image before launch. The API does not interactively start the auth proxy. Keep real API keys and OAuth tokens out of `LaunchRequest` and the VM; keep the admin secret in host `Options` only. The product applies its own host restrictions and authorizes users, cases, and runs. For case credentials and execution identity, see [case-scoped credentials](credential-store.md).

## Retries and recovery

Make each `RequestID` unique across the node. Include the user or case, project, run, attempt, and connection version when those distinctions matter to your product. Reusing an ID with different immutable options returns `ErrConflict`. The node stores hashes of the ID and settings in a durable ledger, not the original ID, task text, or proxy admin secret.

With `UseWorktree: true` and no branch name, the API derives a branch name from the request ID. A safe retry can reuse that branch after cleanup of a failed launch.

Concurrent launches with the same ID allow only one launcher to proceed; the other gets `ErrInProgress`. If the response is lost, retry with the same ID to find the existing worker. After a launcher disappears, the node reconciles its ledger with containerd and reservations before allowing a retry. While the outcome is uncertain, it returns `ErrInProgress` and creates no second VM. A completed worker removed later cannot be recreated with the same ID; the API returns `ErrRequestGone`. Use a new ID for a new attempt.

While the five-minute launch lease is valid, an existing worker alone does not mean startup has completed. If the launcher disappears before recording completion, retries may return `ErrInProgress` until the lease expires. Once completion is stored in the ledger, retries return that result.

If `Launch` returns an error and a `Worker.ID`, inspect it with `Get` and clean it up with `Remove`. `ErrOutcomeUnknown` means launch confirmation was lost; retry with the same ID. `ErrCleanupPending` means cleanup must be retried.

`Request(ctx, requestID)` reports the durable `pending`, `complete`, `failed`, or `cleanup-pending` state without exposing the live lease. `RemoveRequest(ctx, requestID, true)` retries cleanup of the associated worker and deletion journal. It also succeeds if only the prior cleanup response was lost. Keep `Options.StateDir` stable across product restarts.

Canceling the `Exec` context stops the guest command. Canceling `Wait` stops only the wait. After a canceled `Launch`, the API still attempts cleanup. Call `Stop` to stop a VM and revoke its token; call `Remove` to release its resources.

## Errors and compatibility

Use `errors.Is` for `ErrNotFound`, `ErrConflict`, `ErrInProgress`, `ErrCleanupPending`, `ErrRequestGone`, `ErrCapacity`, `ErrVersionMismatch`, and `ErrOutcomeUnknown`. Context cancellation and deadline errors are preserved. Use `errors.Unwrap` to inspect the cause.

The public Go signatures do not expose internal node DTOs. Deploy the CLI, library, node, and proxy image from matching builds. The Go node protocol is v2; `Open` returns `ErrVersionMismatch` for incompatible node versions. `Capabilities()` reports the validated node and `ProxyCapabilities(ctx)` reports the connected auth proxy. The node's HTTP/JSON protocol over the Unix socket is internal and has no external compatibility guarantee.

[Additional policy, execution and recovery APIs](sandbox-features.md)

`Create(ctx, request)` creates a stopped Kata worker; `Start(ctx, worker)` boots and verifies it before token activation and hook/agent execution. `Launch` retains its combined create/start behavior. `ResourceUsage(ctx, worker)` separates observed host cgroup current/peak memory and OOM kills from the profile budgets. See [operating conditions](sandbox-features.md#model-authorization-and-staged-startup).
