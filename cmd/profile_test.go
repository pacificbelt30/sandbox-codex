package cmd

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/pacificbelt30/codex-dock/internal/kataguard"
)

func TestProfileJSONMatchesKataLimits(t *testing.T) {
	cmd := newProfileCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var got []kataguard.ResourceProfile
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Name != "standard" || got[0].GuestMemoryBytes != kataguard.GuestMemory || got[0].HostMemoryBytes != kataguard.HostMemory || got[1].Name != "small" || got[1].GuestMemoryBytes != kataguard.SmallGuestMemory || got[2].Name != "compact" || got[2].GuestMemoryBytes != kataguard.CompactGuestMemory || got[2].HostMemoryBytes != kataguard.CompactHostMemory {
		t.Fatalf("profile = %+v", got)
	}
}
