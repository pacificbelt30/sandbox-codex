package networkpolicy

import (
	"context"
	"fmt"
	"net/netip"
	"testing"
)

func BenchmarkDestinationPolicy(b *testing.B) {
	for _, size := range []int{10, 100, 1000} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			rules := Rules{BlockPrivate: true}
			for i := 0; i < size; i++ {
				rules.AllowDomains = append(rules.AllowDomains, fmt.Sprintf("registry-%d.example.com", i))
			}
			p, err := Compile(rules)
			if err != nil {
				b.Fatal(err)
			}
			d := Destination{Host: fmt.Sprintf("cdn.registry-%d.example.com", size-1), Port: 443}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := p.CheckDestination(context.Background(), d); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkAddressPolicy(b *testing.B) {
	p, err := Compile(Rules{BlockPrivate: true})
	if err != nil {
		b.Fatal(err)
	}
	d := Destination{Host: "registry.example.com", Port: 443}
	ip := netip.MustParseAddr("203.0.113.10")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if err := p.CheckAddress(context.Background(), d, ip); err != nil {
			b.Fatal(err)
		}
	}
}
