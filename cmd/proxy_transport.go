package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
)

const proxyTransportLabel = "io.codex-dock.proxy-transport"

func proxyTransportID(a proxyRunArgs) string {
	if a.upstreamProxy == "" && a.caFile == "" && a.brokerFile == "" {
		return ""
	}
	bundle, _ := os.ReadFile(a.caFile)
	broker, _ := os.ReadFile(a.brokerFile)
	bundle = append(bundle, broker...)
	sum := sha256.Sum256(append([]byte(a.upstreamProxy+"\x00"), bundle...))
	return hex.EncodeToString(sum[:])
}
