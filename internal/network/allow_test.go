package network

import "testing"

func TestParseHostEndpoint(t *testing.T) {
	tests := []struct {
		name string
		spec string
		want HostEndpoint
		ok   bool
	}{
		{name: "ipv4", spec: "203.0.113.10:8080", want: HostEndpoint{IP: "203.0.113.10", Port: 8080}, ok: true},
		{name: "ipv4 trimmed", spec: "  203.0.113.10:443 ", want: HostEndpoint{IP: "203.0.113.10", Port: 443}, ok: true},
		{name: "ipv6 bracketed", spec: "[2001:db8::1]:8080", want: HostEndpoint{IP: "2001:db8::1", Port: 8080}, ok: true},
		{name: "empty", spec: "", ok: false},
		{name: "no port", spec: "203.0.113.10", ok: false},
		{name: "hostname rejected", spec: "example.com:443", ok: false},
		{name: "port zero", spec: "203.0.113.10:0", ok: false},
		{name: "port too large", spec: "203.0.113.10:70000", ok: false},
		{name: "non-numeric port", spec: "203.0.113.10:http", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseHostEndpoint(tt.spec)
			if tt.ok {
				if err != nil {
					t.Fatalf("ParseHostEndpoint(%q) unexpected error: %v", tt.spec, err)
				}
				if got != tt.want {
					t.Fatalf("ParseHostEndpoint(%q) = %+v, want %+v", tt.spec, got, tt.want)
				}
				return
			}
			if err == nil {
				t.Fatalf("ParseHostEndpoint(%q) = %+v, want error", tt.spec, got)
			}
		})
	}
}

func TestParseHostEndpoints(t *testing.T) {
	got, err := ParseHostEndpoints([]string{"203.0.113.10:8080", "198.51.100.5:443"})
	if err != nil {
		t.Fatalf("ParseHostEndpoints() unexpected error: %v", err)
	}
	want := []HostEndpoint{
		{IP: "203.0.113.10", Port: 8080},
		{IP: "198.51.100.5", Port: 443},
	}
	if len(got) != len(want) {
		t.Fatalf("ParseHostEndpoints() len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ParseHostEndpoints()[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}

	if _, err := ParseHostEndpoints([]string{"203.0.113.10:8080", "bad-entry"}); err == nil {
		t.Fatalf("ParseHostEndpoints() with invalid entry: want error, got nil")
	}
}
