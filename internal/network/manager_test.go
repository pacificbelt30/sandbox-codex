package network

import (
	"testing"

	dockernetwork "github.com/docker/docker/api/types/network"
)

func TestDeriveGateway(t *testing.T) {
	tests := []struct {
		cidr    string
		want    string
		wantErr bool
	}{
		{"192.168.200.0/24", "192.168.200.1", false},
		{"10.0.0.0/8", "10.0.0.1", false},
		{"172.16.0.0/12", "172.16.0.1", false},
		{"192.168.1.0/24", "192.168.1.1", false},
		{"invalid", "", true},
		{"256.0.0.0/24", "", true}, // octet out of range
		{"192.168.1/24", "", true}, // too few octets
	}

	for _, tt := range tests {
		t.Run(tt.cidr, func(t *testing.T) {
			got, err := deriveGateway(tt.cidr)
			if tt.wantErr {
				if err == nil {
					t.Errorf("deriveGateway(%q) expected error, got %q", tt.cidr, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("deriveGateway(%q) unexpected error: %v", tt.cidr, err)
			}
			if got != tt.want {
				t.Errorf("deriveGateway(%q) = %q; want %q", tt.cidr, got, tt.want)
			}
		})
	}
}

func TestParseIPv4Network(t *testing.T) {
	tests := []struct {
		cidr    string
		want    [4]byte
		wantErr bool
	}{
		{"192.168.200.0/24", [4]byte{192, 168, 200, 0}, false},
		{"10.0.0.0/8", [4]byte{10, 0, 0, 0}, false},
		{"no-slash", [4]byte{}, true},
		{"256.0.0.0/24", [4]byte{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.cidr, func(t *testing.T) {
			got, err := parseIPv4Network(tt.cidr)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parseIPv4Network(%q) expected error", tt.cidr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseIPv4Network(%q) unexpected error: %v", tt.cidr, err)
			}
			if got != tt.want {
				t.Errorf("parseIPv4Network(%q) = %v; want %v", tt.cidr, got, tt.want)
			}
		})
	}
}

func TestAllowHostEndpoint(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want HostEndpoint
		ok   bool
	}{
		{
			name: "literal ip",
			raw:  "http://192.168.1.9:18080",
			want: HostEndpoint{IP: "192.168.1.9", Port: 18080},
			ok:   true,
		},
		{
			name: "hostname ignored",
			raw:  "http://host.docker.internal:18080",
			ok:   false,
		},
		{
			name: "invalid url",
			raw:  "://bad",
			ok:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := AllowHostEndpoint(tt.raw)
			if ok != tt.ok {
				t.Fatalf("AllowHostEndpoint(%q) ok=%v want=%v", tt.raw, ok, tt.ok)
			}
			if got != tt.want {
				t.Fatalf("AllowHostEndpoint(%q)=%+v want=%+v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestGatewayFromSummary(t *testing.T) {
	net := &dockernetwork.Summary{
		IPAM: dockernetwork.IPAM{
			Config: []dockernetwork.IPAMConfig{
				{Subnet: "10.200.0.0/24"},
			},
		},
	}

	if got := gatewayFromSummary(net); got != "10.200.0.1" {
		t.Fatalf("gatewayFromSummary()=%q want 10.200.0.1", got)
	}
}
