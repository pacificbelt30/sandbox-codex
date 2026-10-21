package sandbox

import (
	"encoding/json"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/pacificbelt30/codex-dock/internal/workerpolicy"
)

func TestTemporaryStorageRequiresNewNodeBeforeSideEffects(t *testing.T) {
	f, m := newEngineFixture(t)
	_, err := m.Run(RunOptions{Name: "unsupported-tmpfs", ProjectDir: t.TempDir(), Detach: true, TemporaryStorage: []workerpolicy.TemporaryStorage{{Target: "/scratch", MaxBytes: 8 << 20}}})
	if err == nil || f.worker != nil || f.front != nil || f.starts != 0 {
		t.Fatalf("unsupported storage had side effects: %v", err)
	}
}

func TestTemporaryStorageCannotShadowReadonlyOrDeclaredMounts(t *testing.T) {
	for _, tc := range []struct {
		target, bind                  string
		readonly, root, denied, valid bool
	}{
		{"/scratch", "/workspace", true, true, false, true},
		{"/workspace/build", "/workspace", true, false, false, false},
		{"/workspace/build", "/workspace", false, false, false, true},
		{"/workspace/build", "/workspace", false, false, true, false},
		{"/data", "/data/child", false, false, false, false},
		{"/data/child", "/data", true, false, false, false},
		{"/var", "/workspace", false, true, false, false},
	} {
		var denied []string
		if tc.denied {
			denied = []string{"/workspace/build/config"}
		}
		err := validateTemporaryTopology([]workerpolicy.TemporaryStorage{{Target: tc.target}}, []mount.Mount{{Target: tc.bind, ReadOnly: tc.readonly}}, denied, tc.root)
		if (err == nil) != tc.valid {
			t.Fatalf("target %s bind %s: %v", tc.target, tc.bind, err)
		}
	}
}

func TestSavedTemporaryStorageAndGuestAttestationRejectDrift(t *testing.T) {
	storage, err := workerpolicy.NormalizeTemporaryStorage([]workerpolicy.TemporaryStorage{{Target: "/scratch", MaxBytes: 8 << 20}})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(storage)
	if err != nil {
		t.Fatal(err)
	}
	v := container.InspectResponse{Config: &container.Config{Labels: map[string]string{workerpolicy.TemporaryStorageLabel: string(raw)}}, ContainerJSONBase: &container.ContainerJSONBase{HostConfig: &container.HostConfig{Tmpfs: map[string]string{"/scratch": storage[0].MountOptions(), "/run/codex-dock-control": "bootstrap"}}}}
	if _, err := validateSavedTemporaryStorage(v, 512<<20); err != nil {
		t.Fatal(err)
	}
	v.HostConfig.Tmpfs["/scratch"] = "rw,size=16777216"
	if _, err := validateSavedTemporaryStorage(v, 512<<20); err == nil {
		t.Fatal("retained quota drift accepted")
	}
	good := workerpolicy.TemporaryObservation{Target: "/scratch", Filesystem: 0x01021994, MaxBytes: 8 << 20, UID: 1000, GID: 1000, Mode: 0700, Flags: 2 | 4 | 8}
	encode := func(values []workerpolicy.TemporaryObservation) []byte {
		t.Helper()
		observations, err := json.Marshal(values)
		if err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(map[string]string{"status": "Uid:\t1000 1000 1000 1000\nGid:\t1000 1000 1000 1000\n", "temporary_storage": string(observations)})
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	if err := verifyTemporaryGuest(encode([]workerpolicy.TemporaryObservation{good}), storage, true); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*workerpolicy.TemporaryObservation){func(v *workerpolicy.TemporaryObservation) { v.MaxBytes++ }, func(v *workerpolicy.TemporaryObservation) { v.UID = 0 }, func(v *workerpolicy.TemporaryObservation) { v.Flags = 2 | 4 }, func(v *workerpolicy.TemporaryObservation) { v.Flags |= 1 }, func(v *workerpolicy.TemporaryObservation) { v.Filesystem = 0 }, func(v *workerpolicy.TemporaryObservation) { v.Mode = 0777 }, func(v *workerpolicy.TemporaryObservation) { v.Target = "/alias" }} {
		v := good
		mutate(&v)
		if err := verifyTemporaryGuest(encode([]workerpolicy.TemporaryObservation{v}), storage, true); err == nil {
			t.Fatal("unverified guest temporary filesystem accepted")
		}
	}
	good.Mode = 0755
	if err := verifyTemporaryGuest(encode([]workerpolicy.TemporaryObservation{good}), storage, false); err != nil {
		t.Fatal("ordinary chmod after startup blocked later exec:", err)
	}
}
