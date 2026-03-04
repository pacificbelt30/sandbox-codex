package network

import "testing"

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
		{"192.168.1/24", "", true},  // too few octets
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
