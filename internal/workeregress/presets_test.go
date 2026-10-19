package workeregress

import (
	"github.com/pacificbelt30/codex-dock/internal/networkpolicy"
	"testing"
)

func TestPresetsAreOptInAndKeepHostCeiling(t *testing.T) {
	host := networkpolicy.Rules{AllowDomains: []string{"example.com"}, DenyPorts: []int{22}, BlockPrivate: true}
	r, off, err := ResolvePreset(host, "", nil)
	if err != nil || off || ID(r) != ID(host) {
		t.Fatalf("legacy changed: %+v %v", r, err)
	}
	if _, _, err = ResolvePreset(host, "dependencies", nil); err == nil {
		t.Fatal("preset widened host allowlist")
	}
	r, off, err = ResolvePreset(host, "api", []string{"api.example.com"})
	if err != nil || off || len(r.DenyPorts) != 1 {
		t.Fatal("host restriction lost")
	}
	if _, off, err = ResolvePreset(host, "offline", nil); err != nil || !off {
		t.Fatal(err)
	}
	if _, _, err = ResolvePreset(host, "api", nil); err == nil {
		t.Fatal("API preset accepted no domains")
	}
}
