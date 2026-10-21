package kataguard

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

const guestPIDPolicy = "oci-annotation-v1"

// Bind capability discovery to the prepared image hash, then pin this metadata
// as an asset. Legacy images and manifests remain valid for 512-PID profiles.
func readGuestPolicy(path, imageHash string) (bool, error) {
	if err := trustedPath(path, false); err != nil {
		return false, err
	}
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, (64<<10)+1))
	if err != nil {
		return false, err
	}
	if len(data) > 64<<10 {
		return false, fmt.Errorf("guest policy metadata too large")
	}
	var policy struct {
		SchemaVersion int               `json:"schema_version"`
		PIDLimit      int64             `json:"pids_limit"`
		PIDPolicy     string            `json:"pid_policy"`
		Files         map[string]string `json:"files"`
	}
	if err := json.Unmarshal(data, &policy); err != nil {
		return false, err
	}
	if policy.PIDLimit != 512 || policy.Files["guest.image"] != imageHash {
		return false, fmt.Errorf("guest PID policy is not bound to the pinned image")
	}
	if policy.SchemaVersion == 1 && policy.PIDPolicy == "" {
		return false, nil
	}
	if policy.SchemaVersion != 2 || policy.PIDPolicy != guestPIDPolicy {
		return false, fmt.Errorf("unsupported guest PID policy")
	}
	return true, nil
}

func (g *Guard) supportsPIDProfiles() bool { _, ok := g.manifest.Assets["guest-policy"]; return ok }
