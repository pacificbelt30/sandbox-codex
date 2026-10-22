package sandbox

import (
	"testing"

	"github.com/pacificbelt30/codex-dock/internal/authproxy"
)

type profileRuntimeProxy struct {
	runtimeProxy
	profile, connection string
}

func (p *profileRuntimeProxy) AuthProfile() string              { return p.profile }
func (p *profileRuntimeProxy) ProfileConnectionVersion() string { return p.connection }

func TestWorkerRestartBindsProfileAndConnection(t *testing.T) {
	f, m := newEngineFixture(t)
	p := &profileRuntimeProxy{profile: "work", connection: "login-1"}
	m.proxy = p
	_, err := m.Run(RunOptions{Name: "profile-worker", ProjectDir: t.TempDir(), Runtime: RuntimeKata, Detach: true})
	if err != nil {
		t.Fatal(err)
	}
	labels := f.worker.Config.Labels
	if labels[labelPrefix+"auth-profile"] != "work" || labels[labelPrefix+"auth-profile-connection"] != "login-1" {
		t.Fatal("worker lost profile binding")
	}
	before := len(p.restored)
	if err := m.restoreWorkerToken(*f.worker); err != nil || len(p.restored) != before+1 {
		t.Fatalf("same login could not restart: %v", err)
	}
	for _, change := range []func(){
		func() { p.profile = "other" },
		func() { p.profile = "work"; p.connection = "login-2" },
		func() { p.profile = ""; p.connection = "" },
	} {
		change()
		if err := m.restoreWorkerToken(*f.worker); err == nil {
			t.Fatal("restart silently changed login")
		}
	}
	if len(p.restored) != before+1 {
		t.Fatal("mismatched profile reauthorized old placeholder")
	}
	for _, tc := range []struct {
		p      authproxy.Service
		labels map[string]string
		valid  bool
	}{
		{&runtimeProxy{}, nil, true},
		{&profileRuntimeProxy{profile: "work", connection: "login-1"}, nil, false},
		{&runtimeProxy{}, labels, false},
		{&profileRuntimeProxy{profile: "work", connection: "login-1"}, map[string]string{labelPrefix + "auth-profile": "work"}, false},
	} {
		if (checkWorkerProfile(tc.p, tc.labels) == nil) != tc.valid {
			t.Fatal("legacy/profile restart binding mismatch")
		}
	}
}
