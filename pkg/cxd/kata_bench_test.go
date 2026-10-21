package cxd

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/pacificbelt30/codex-dock/internal/engine"
)

// Reuse an idle managed worker to measure the public Exec path, including all
// per-exec host, asset and guest checks. Creation and Open are outside timing.
// The caller owns the worker's lifecycle; this benchmark only runs /bin/true.
func BenchmarkKataExec(b *testing.B) {
	id := os.Getenv("CODEX_DOCK_BENCH_WORKER")
	if id == "" {
		b.Skip("set CODEX_DOCK_BENCH_WORKER to an idle managed Kata worker ID")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	client, err := Open(ctx, Options{NodeSocket: os.Getenv("CODEX_DOCK_KATA_NODE_SOCKET"), StateDir: b.TempDir()})
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = client.Close() }()
	node, err := engine.NewClient()
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = node.Close() }()
	worker, err := node.ContainerInspect(ctx, id)
	if err != nil {
		b.Fatal(err)
	}
	if worker.Config == nil || worker.Config.Labels["codex-dock.isolation"] != "kata" {
		b.Fatal("benchmark requires a Kata worker")
	}
	command := ExecRequest{Argv: []string{"/bin/true"}}
	// Warm the manager connection and runtime's first exec outside timing.
	if code, err := client.Exec(ctx, id, command); err != nil || code != 0 {
		b.Fatalf("warmup exec: exit=%d error=%v", code, err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if code, err := client.Exec(ctx, id, command); err != nil || code != 0 {
			b.Fatalf("exec: exit=%d error=%v", code, err)
		}
	}
	b.StopTimer()
}
