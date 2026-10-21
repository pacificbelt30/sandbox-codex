package workerpolicy

import "testing"

func TestTemporaryPoliciesNormalizeWithoutWeakeningRestrictions(t *testing.T) {
	input := []TemporaryStorage{{Target: "/run/build-cache", MaxBytes: 8 << 20}}
	policies, err := NormalizeTemporaryStorage(input)
	if err != nil {
		t.Fatal(err)
	}
	if policies[0].Medium != StorageMemory || policies[0].Lifetime != StorageLifetimeVM || input[0].Medium != "" {
		t.Fatal("defaults mutated the caller's request")
	}
	if err := ValidateTemporaryBudget(policies, 4<<20); err == nil {
		t.Fatal("unfulfillable guest budget accepted")
	}
	for _, target := range []string{"/", "/run", "/proc/hidden", "/sys", "/dev/shm", "/etc", "/run/codex-dock-guard", "/run/codex-dock-control/subdir", "relative", "/run/../scratch", "/scratch/"} {
		if _, err := NormalizeTemporaryStorage([]TemporaryStorage{{Target: target, MaxBytes: 4096}}); err == nil {
			t.Fatalf("unsafe path accepted: %s", target)
		}
	}
	for _, v := range []TemporaryStorage{{Target: "/scratch", MaxBytes: 4097}, {Target: "/scratch", MaxBytes: 4096, Medium: "disk"}, {Target: "/scratch", MaxBytes: 4096, Lifetime: "persistent"}} {
		if _, err := NormalizeTemporaryStorage([]TemporaryStorage{v}); err == nil {
			t.Fatal("unsupported guarantee silently accepted")
		}
	}
	if _, err := NormalizeTemporaryStorage([]TemporaryStorage{{Target: "/scratch", MaxBytes: 4096}, {Target: "/scratch/child", MaxBytes: 4096}}); err == nil {
		t.Fatal("overlapping ceilings accepted")
	}
}
