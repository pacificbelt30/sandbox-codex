# Performance and benchmarks

[日本語](../performance.md) | **English**

Measure Kata asset verification, public Go API Exec and proxy authentication separately.
The [measurement and regression record](../testing.md#2026-10-21の性能計測) (Japanese) contains the before/after values, conditions and verified scope.

## How the optimizations work

Asset verification checks file ownership, paths, executable permissions and SHA-256 with at most `min(4, GOMAXPROCS)` concurrent readers per validation.
Every validation reads every asset and checks configuration bindings. This uses spare CPU capacity while retaining the [security checks](security.md).
The total hashing work is unchanged. Measure throughput under CPU saturation and performance on storage-bound hosts separately.

The auth proxy looks up issuance records directly by token value.
Issuance, restore and revocation update the index under the same lock, avoiding a scan of every worker.
Expiry, provider/model restrictions and active-stream cancellation follow the [auth proxy contract](auth-proxy.md).

These optimizations are enabled by the implementation and require no activation flag.
Follow the [CLI, node and proxy update procedure](../operations.md#cliプロキシを更新する) (Japanese). Products using the Go API must also use the same source version as the node.

## Choose a measurement

| Benchmark | Includes | Excludes |
|---|---|---|
| `BenchmarkWorkerRequest` | Authentication header parsing, record lookup, expiry/route checks, request context creation and release | Network and upstream API communication |
| `BenchmarkLoadPinnedManifest` | Reading real assets; checksum, ownership, path and configuration checks | VM startup and guest execution |
| `BenchmarkKataExec` | Per-exec asset, host and guest checks; running `/bin/true` and checking completion | VM creation, client Open, warmup and AI inference |

The proxy benchmark measures accepted and unissued tokens with 1, 32, 256 and 4,096 issuance records.
Allocation figures reported by the Exec benchmark cover the client process.

## Run the benchmarks

Run from the repository root with the [development Go environment](../testing.md#コードを確認する) (Japanese).
The following `CODEX_DOCK_BENCH_*` variables are inputs to benchmarks only.

### Proxy authentication

No Kata node or real upstream API connection is required.

```bash
go test ./internal/authproxy -run '^$' \
  -bench '^BenchmarkWorkerRequest$' -benchmem -count=5 -benchtime=500ms
```

### Pinned asset verification

On Linux, specify an existing sealed manifest. The benchmark reads files without modifying them.
A root-owned manifest requires the node operator's read permissions. Run from trusted source.

```bash
CODEX_DOCK_BENCH_MANIFEST=/etc/codex-dock/kata-assets.json \
  go test ./internal/kataguard -run '^$' \
    -bench '^BenchmarkLoadPinnedManifest$' -benchmem -benchtime=3x -count=5
```

### Exec in a real Kata VM

On a prepared test host, start an idle managed Kata worker through the normal workflow and obtain its full ID.
The benchmark runs `/bin/true` in that VM. The caller starts the worker and stops/removes it after measurement.
Replace `FULL_WORKER_ID` with its actual ID and set the `dev` value in `-ldflags` to the running node's BuildVersion.

```bash
CODEX_DOCK_KATA_NODE_SOCKET=/run/codex-dock-node.sock \
CODEX_DOCK_BENCH_WORKER=FULL_WORKER_ID \
  go test -ldflags '-X github.com/pacificbelt30/codex-dock/internal/buildinfo.Version=dev' \
    ./pkg/cxd -run '^$' -bench '^BenchmarkKataExec$' -benchmem -benchtime=3x -count=5
```

Omitting the manifest path or worker ID skips the corresponding benchmark. A skip is not a successful measurement.

## Compare revisions

Use the same Go version, CPU conditions, assets and worker profile, with no competing builds or tests.
Build the node and client with the same measurement BuildVersion. Alternate the order of old/new revisions within pairs and compare medians across samples.
Distinguish warm page-cache measurements from runs that include initial disk reads.

Run [regression checks](../testing.md) (Japanese) after measurement. Runtime changes require `make kata-e2e`. Authentication changes also need active-connection cancellation checks for revocation, expiry, replacement, restore and proxy stop.
Remove test VMs, containers and networks before deleting test services or ownership records.
