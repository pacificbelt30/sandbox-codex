package network

import (
	"errors"
	"strings"
	"testing"
)

func TestWorkerNetworkName(t *testing.T) {
	tests := []struct {
		worker string
		want   string
	}{
		{"alpha", "dock-net-w-alpha"},
		{"worker-1", "dock-net-w-worker-1"},
	}
	for _, tt := range tests {
		if got := WorkerNetworkName(tt.worker); got != tt.want {
			t.Errorf("WorkerNetworkName(%q) = %q; want %q", tt.worker, got, tt.want)
		}
	}
	if !strings.HasPrefix(WorkerNetworkName("x"), WorkerNetPrefix) {
		t.Errorf("WorkerNetworkName should start with %q", WorkerNetPrefix)
	}
}

func TestProxyNetworkAliases(t *testing.T) {
	if ProxyNetworkName != EgressNetworkName {
		t.Errorf("ProxyNetworkName=%q should alias EgressNetworkName=%q", ProxyNetworkName, EgressNetworkName)
	}
	if ProxyBridgeName != EgressBridgeName {
		t.Errorf("ProxyBridgeName=%q should alias EgressBridgeName=%q", ProxyBridgeName, EgressBridgeName)
	}
}

func TestIsAlreadyConnected(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{errors.New("endpoint with name codex-auth-proxy already exists in network dock-net-w-a"), true},
		{errors.New("container is already connected to network"), true},
		{errors.New("some other error"), false},
	}
	for _, tt := range tests {
		if got := isAlreadyConnected(tt.err); got != tt.want {
			t.Errorf("isAlreadyConnected(%q) = %v; want %v", tt.err, got, tt.want)
		}
	}
}

func TestIsNotConnected(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{errors.New("container codex-auth-proxy is not connected to network dock-net-w-a"), true},
		{errors.New("No such network: dock-net-w-a"), true},
		{errors.New("network dock-net-w-a not found"), true},
		{errors.New("permission denied"), false},
	}
	for _, tt := range tests {
		if got := isNotConnected(tt.err); got != tt.want {
			t.Errorf("isNotConnected(%q) = %v; want %v", tt.err, got, tt.want)
		}
	}
}
