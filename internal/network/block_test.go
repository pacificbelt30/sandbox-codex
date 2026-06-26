package network

import "testing"

func TestParseBlockDestination(t *testing.T) {
	tests := []struct {
		name string
		spec string
		want BlockDestination
		ok   bool
	}{
		{name: "cidr", spec: "203.0.113.0/24", want: BlockDestination{CIDR: "203.0.113.0/24", Port: 0}, ok: true},
		{name: "cidr normalized", spec: "203.0.113.10/24", want: BlockDestination{CIDR: "203.0.113.0/24", Port: 0}, ok: true},
		{name: "bare ip", spec: "203.0.113.10", want: BlockDestination{CIDR: "203.0.113.10/32", Port: 0}, ok: true},
		{name: "ip port", spec: "203.0.113.10:443", want: BlockDestination{CIDR: "203.0.113.10/32", Port: 443}, ok: true},
		{name: "trimmed", spec: "  203.0.113.10:80 ", want: BlockDestination{CIDR: "203.0.113.10/32", Port: 80}, ok: true},
		{name: "empty", spec: "", ok: false},
		{name: "ipv6 cidr rejected", spec: "2001:db8::/32", ok: false},
		{name: "ipv6 ip rejected", spec: "2001:db8::1", ok: false},
		{name: "hostname rejected", spec: "example.com", ok: false},
		{name: "bad cidr", spec: "203.0.113.0/99", ok: false},
		{name: "port zero", spec: "203.0.113.10:0", ok: false},
		{name: "port too large", spec: "203.0.113.10:70000", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseBlockDestination(tt.spec)
			if tt.ok {
				if err != nil {
					t.Fatalf("ParseBlockDestination(%q) unexpected error: %v", tt.spec, err)
				}
				if got != tt.want {
					t.Fatalf("ParseBlockDestination(%q) = %+v, want %+v", tt.spec, got, tt.want)
				}
				return
			}
			if err == nil {
				t.Fatalf("ParseBlockDestination(%q) = %+v, want error", tt.spec, got)
			}
		})
	}
}

func TestParseBlockDestinations(t *testing.T) {
	got, err := ParseBlockDestinations([]string{"203.0.113.0/24", "198.51.100.5:443"})
	if err != nil {
		t.Fatalf("ParseBlockDestinations() unexpected error: %v", err)
	}
	want := []BlockDestination{
		{CIDR: "203.0.113.0/24", Port: 0},
		{CIDR: "198.51.100.5/32", Port: 443},
	}
	if len(got) != len(want) {
		t.Fatalf("ParseBlockDestinations() len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ParseBlockDestinations()[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}

	if _, err := ParseBlockDestinations([]string{"203.0.113.0/24", "nope"}); err == nil {
		t.Fatalf("ParseBlockDestinations() with invalid entry: want error, got nil")
	}
}

func TestNormalizeBlockDestinations(t *testing.T) {
	got := normalizeBlockDestinations([]BlockDestination{
		{CIDR: "203.0.113.0/24", Port: 0},
		{CIDR: "203.0.113.0/24", Port: 0},
		{CIDR: "", Port: 80},
		{CIDR: "198.51.100.5/32", Port: 70000},
		{CIDR: "198.51.100.5/32", Port: 443},
	})

	want := []BlockDestination{
		{CIDR: "198.51.100.5/32", Port: 443},
		{CIDR: "203.0.113.0/24", Port: 0},
	}
	if len(got) != len(want) {
		t.Fatalf("normalizeBlockDestinations() len = %d, want %d\n%+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("normalizeBlockDestinations()[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}
