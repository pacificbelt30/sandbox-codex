//go:build linux

package kataguard

import (
	"os"
	"testing"
)

// Use an existing sealed manifest read-only. This includes path, ownership,
// content checksum and config validation with real asset sizes. The benchmark
// may require the node operator's permissions to read the manifest.
func BenchmarkLoadPinnedManifest(b *testing.B) {
	path := os.Getenv("CODEX_DOCK_BENCH_MANIFEST")
	if path == "" {
		b.Skip("set CODEX_DOCK_BENCH_MANIFEST to an existing sealed manifest")
	}
	if _, err := Load(path); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Load(path); err != nil {
			b.Fatal(err)
		}
	}
}
