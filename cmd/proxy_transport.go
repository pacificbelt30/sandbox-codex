package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
)

const proxyTransportLabel = "io.codex-dock.proxy-transport"

func proxyTransportID(a proxyRunArgs) string {
	if a.upstreamProxy == "" && a.caFile == "" && a.brokerFile == "" && len(a.allowedModels) == 0 {
		return ""
	}
	bundle, _ := os.ReadFile(a.caFile)
	broker, _ := os.ReadFile(a.brokerFile)
	bundle = append(bundle, broker...)
	if len(a.allowedModels) != 0 {
		models, _ := json.Marshal(a.allowedModels)
		bundle = append(bundle, models...)
	}
	sum := sha256.Sum256(append([]byte(a.upstreamProxy+"\x00"), bundle...))
	return hex.EncodeToString(sum[:])
}
