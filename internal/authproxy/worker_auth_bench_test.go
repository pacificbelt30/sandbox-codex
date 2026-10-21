package authproxy

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Measure the complete authorization path, including request context ownership,
// rather than only a map lookup. No network or upstream credentials are used.
func BenchmarkWorkerRequest(b *testing.B) {
	for _, count := range []int{1, 32, 256, 4096} {
		b.Run(fmt.Sprintf("workers=%d", count), func(b *testing.B) {
			p, err := NewProxy(Config{TokenTTL: 3600})
			if err != nil {
				b.Fatal(err)
			}
			b.Cleanup(p.Stop)
			tokens := make([]string, count)
			for i := range tokens {
				tokens[i], err = p.IssueToken(fmt.Sprintf("worker-%d", i), 3600)
				if err != nil {
					b.Fatal(err)
				}
			}
			for _, valid := range []bool{true, false} {
				b.Run(fmt.Sprintf("valid=%t", valid), func(b *testing.B) {
					requests := make([]*http.Request, count)
					for i, token := range tokens {
						if !valid {
							token = "cdx-unissued"
						}
						requests[i] = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
						requests[i].Header.Set("Authorization", "Bearer "+token)
					}
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						_, release, ok := p.workerRequest(requests[i%count], false)
						if ok != valid {
							b.Fatal("unexpected authorization result")
						}
						if release != nil {
							release()
						}
					}
				})
			}
		})
	}
}
