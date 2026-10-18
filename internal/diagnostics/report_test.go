package diagnostics

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestReportPreservesIndependentFailures(t *testing.T) {
	var r Report
	r.Add("kvm", "available", "enable KVM", nil)
	r.Add("node", "", "start node", errors.New("connection refused"))
	r.Add("builder", "", "start builder", errors.New("socket missing"))
	if err := r.Err(); err == nil || !strings.Contains(err.Error(), "node, builder") {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := r.Write(&out, true); err != nil {
		t.Fatal(err)
	}
	var decoded Report
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Checks) != 3 || decoded.Checks[1].Fix != "start node" || decoded.Checks[0].Fix != "" {
		t.Fatal(decoded)
	}
	out.Reset()
	if err := r.Write(&out, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Fix: start builder") {
		t.Fatal(out.String())
	}
}
func TestHintsKeepAssetChecksAndSocketPermissions(t *testing.T) {
	for _, s := range []string{"shared workspaces require virtio_fs_cache", "asset checksum differs", "manifest changed"} {
		if !strings.Contains(Hint(errors.New(s)), "setup") {
			t.Fatal(s)
		}
	}
	if !strings.Contains(Hint(errors.New("permission denied")), "log in again") {
		t.Fatal("missing group advice")
	}
}
