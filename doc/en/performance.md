# Performance and benchmarks

[日本語](../performance.md) | **English**

- Measure Kata asset verification, public Go API Exec and proxy authentication separately.
- The [measurement and regression record](../testing.md#2026-10-21の性能計測) (Japanese) contains the before/after values, conditions and verified scope.

## How the optimizations work

- Asset verification checks file ownership, paths, executable permissions and SHA-256 with at most `min(4, GOMAXPROCS)` concurrent readers per validation.
- Every validation reads every asset and checks configuration bindings.
- This uses spare CPU capacity while retaining the [security checks](security.md).
- The total hashing work is unchanged.
- Measure throughput under CPU saturation and performance on storage-bound hosts separately.

- The auth proxy looks up issuance records directly by token value.
- Issuance, restore and revocation update the index under the same lock, avoiding a scan of every worker.
- Expiry, provider/model restrictions and active-stream cancellation follow the [auth proxy contract](auth-proxy.md).

- These optimizations are enabled by the implementation and require no activation flag.
- Follow the [CLI, node and proxy update procedure](../operations.md#cliプロキシを更新する) (Japanese).
- Products using the Go API must also use the same source version as the node.

## Choose a measurement

| Benchmark | Includes | Excludes |
|---|---|---|
| `BenchmarkWorkerRequest` | Authentication header parsing, record lookup, expiry/route checks, request context creation and release | Network and upstream API communication |
| `BenchmarkLoadPinnedManifest` | Reading real assets; checksum, ownership, path and configuration checks | VM startup and guest execution |
| `BenchmarkKataExec` | Per-exec asset, host and guest checks; running `/bin/true` and checking completion | VM creation, client Open, warmup and AI inference |
| `BenchmarkKataLaunch` | A fresh Launch and its sequential phases; first Exec is reported separately | Client Open, proxy creation, image builds/imports, removal and AI inference |

- The proxy benchmark measures accepted and unissued tokens with 1, 32, 256 and 4,096 issuance records.
- Allocation figures reported by the Exec benchmark cover the client process.

## Locate startup costs

- The Go API `client.LaunchWithTimings(ctx, req)` returns `Worker`, `LaunchTimings` and `error`.
- It uses the same request identity, verification, authentication and cleanup as `Launch`.
- Measurement does not affect the request fingerprint.
- Ordinary `Launch` does not allocate a timing record.

```go
worker, timing, err := client.LaunchWithTimings(ctx, req)
// Preserve worker.ID for recovery on failure, following the existing API contract.
for _, phase := range timing.Phases {
    fmt.Printf("%s: %s\n", phase.Name, phase.Duration)
}
```

- Phases partition elapsed wall time without overlap; their sum equals `timing.Duration`.
- JSON durations are in nanoseconds.
- They distinguish preflight, network setup, capacity reservation, container creation, created-worker verification, VM start, host/guest verification, token activation, bootstrap release, readiness and request confirmation.
- `vm_start` covers the entire node start API, not pure VMM boot time.
- `host_guest_verification` includes asset revalidation, host checks and the guest probe.
- An idempotent retry reports only the phases actually executed, such as existing-worker lookup.
- Failures return partial measurements including cleanup and request-abort confirmation.

- Reports contain fixed phase names and durations only, without paths, arguments, request values, tokens or error text.
- The caller decides whether to store or transmit them.
- Phase names are diagnostics and may change between versions.
- Use the existing API outcome for readiness and authentication decisions.

## Run the benchmarks

- Run from the repository root with the [development Go environment](../testing.md#コードを確認する) (Japanese).
- The following `CODEX_DOCK_BENCH_*` variables are inputs to benchmarks only.

### Proxy authentication

No Kata node or real upstream API connection is required.

```bash
go test ./internal/authproxy -run '^$' \
  -bench '^BenchmarkWorkerRequest$' -benchmem -count=5 -benchtime=500ms
```

### Pinned asset verification

- On Linux, specify an existing sealed manifest.
- The benchmark reads files without modifying them.
- A root-owned manifest requires the node operator's read permissions.
- Run from trusted source.

```bash
CODEX_DOCK_BENCH_MANIFEST=/etc/codex-dock/kata-assets.json \
  go test ./internal/kataguard -run '^$' \
    -bench '^BenchmarkLoadPinnedManifest$' -benchmem -benchtime=3x -count=5
```

### Exec in a real Kata VM

- On a prepared test host, start an idle managed Kata worker through the normal workflow and obtain its full ID.
- The benchmark runs `/bin/true` in that VM.
- The caller starts the worker and stops/removes it after measurement.
- Replace `FULL_WORKER_ID` with its actual ID and set the `dev` value in `-ldflags` to the running node's BuildVersion.

```bash
CODEX_DOCK_KATA_NODE_SOCKET=/run/codex-dock-node.sock \
CODEX_DOCK_BENCH_WORKER=FULL_WORKER_ID \
  go test -ldflags '-X github.com/pacificbelt30/codex-dock/internal/buildinfo.Version=dev' \
    ./pkg/cxd -run '^$' -bench '^BenchmarkKataExec$' -benchmem -benchtime=3x -count=5
```

- Omitting the manifest path or worker ID skips the corresponding benchmark.
- A skip is not a successful measurement.

### Fresh startup in real Kata

- Use an isolated test node with no other users or concurrent launches.
- Preload the worker and proxy images.
- The benchmark creates a CredentialStore containing only an invalid API key, a dedicated case proxy, and a fresh worker for every sample, then removes them.
- It uses no real account or upstream AI.
- Workers have no general egress, Git metadata sharing or running agent; they remain idle.

```bash
CODEX_DOCK_KATA_NODE_SOCKET=/run/codex-dock-node.sock \
CODEX_DOCK_BENCH_LAUNCH=1 CODEX_DOCK_BENCH_PROFILE=compact \
CODEX_DOCK_KATA_TEST_IMAGE=codex-dock:latest \
CODEX_DOCK_KATA_PROXY_IMAGE=codex-dock-proxy:latest \
GOMAXPROCS=4 \
  go test -ldflags '-X github.com/pacificbelt30/codex-dock/internal/buildinfo.Version=dev' \
    ./pkg/cxd -run '^$' -bench '^BenchmarkKataLaunch$' -benchtime=3x -count=5 -timeout=20m -v
```

- Match `dev` to the node's BuildVersion.
- Omitting `CODEX_DOCK_BENCH_LAUNCH=1` skips this benchmark.
- `launch_ms/op` and phase-specific `*_ms/op` report launch times.
- `first_exec_ms/op` separately measures the first `/bin/true` execution and completion after launch.
- Each `launch_timing` JSON record contains individual measurements.
- The Go benchmark also performs one calibration launch before its measured iterations.
- Use `CODEX_DOCK_BENCH_PROFILE` to select a profile other than the default `compact`.
- Results with preloaded images and a warm page cache do not establish first image-unpack or real task readiness latency.

## Compare revisions

### Choose the next optimization

- [Fresh-start measurements](../testing.md#新規kata起動の工程別計測) (Japanese) identify VM start as the main wait.
- Prebooting unused VMs is the next candidate.
- Warm pools, memory snapshots and VM forks are not implemented.
- Existing `Create` creates a stopped worker; `Start` still boots its VM.

- A warm pool needs to keep bootstrap closed and authentication inactive during preparation, then recheck assets, host/guest restrictions and policy at assignment.
- It must preserve the current immutable workspace, mounts, UID, network, image, model authorization and execution identity.
- Do not reassign workers to different requests or return used VMs to the pool.
- Count waiting VMs against reservations.
- Verify expiry, cancellation, ownership recovery after node restart and exclusive assignment before enabling it.
- Compare assignment through first Exec against ordinary startup under identical conditions, including verification, replenishment and reserved-memory costs.
- VM start's current share is not an achieved speedup.

### Comparison conditions

- Use the same Go version, CPU conditions, assets and worker profile, with no competing builds or tests.
- Build the node and client with the same measurement BuildVersion.
- Alternate the order of old/new revisions within pairs and compare medians across samples.
- Distinguish warm page-cache measurements from runs that include initial disk reads.

- Run [regression checks](../testing.md) (Japanese) after measurement.
- Runtime changes require `make kata-e2e`.
- Authentication changes also need active-connection cancellation checks for revocation, expiry, replacement, restore and proxy stop.
- Remove test VMs, containers and networks before deleting test services or ownership records.
