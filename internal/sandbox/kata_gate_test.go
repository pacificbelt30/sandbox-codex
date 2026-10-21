package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/docker/docker/api/types/mount"
	"github.com/pacificbelt30/codex-dock/internal/launchtiming"
	"os"
	"strings"
	"testing"
)

func TestGuestVerificationFailureNeverReleasesWorkOrEnablesToken(t *testing.T) {
	for _, failure := range []string{"kernel", "pids", "uid", "capabilities", "no-new-privileges", "bootstrap"} {
		t.Run(failure, func(t *testing.T) {
			f, m := newEngineFixture(t)
			p := &runtimeProxy{fakeProxy: fakeProxy{endpoint: "http://auth:18080"}}
			m.proxy = p
			probe := m.execProbe
			released := false
			m.execProbe = func(ctx context.Context, id string, args []string) ([]byte, error) {
				if args[1] == "release" {
					released = true
					return nil, nil
				}
				data, err := probe(ctx, id, args)
				if err != nil {
					return nil, err
				}
				var fields map[string]string
				if err = json.Unmarshal(data, &fields); err != nil {
					return nil, err
				}
				switch failure {
				case "kernel":
					boot, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
					if err != nil {
						return nil, err
					}
					fields["boot_id"] = strings.TrimSpace(string(boot))
				case "pids":
					fields["pids"] = "max"
				case "uid":
					fields["status"] = strings.ReplaceAll(fields["status"], "1000", "0")
				case "capabilities":
					fields["status"] = strings.Replace(fields["status"], "CapEff:\t0000000000000000", "CapEff:\t0000000000000001", 1)
				case "no-new-privileges":
					fields["status"] = strings.Replace(fields["status"], "NoNewPrivs:\t1", "NoNewPrivs:\t0", 1)
				case "bootstrap":
					fields["cmdline"] = "unverified-user-command"
				}
				return json.Marshal(fields)
			}
			ctx, timings := launchtiming.Begin(context.Background())
			if _, err := m.RunContext(ctx, RunOptions{Name: "worker", ProjectDir: t.TempDir(), Detach: true}); err == nil {
				t.Fatal("invalid guest accepted")
			}
			_, phases := timings.Finish()
			cleanupMeasured := false
			for _, phase := range phases {
				if phase.Name == "token_activation" || phase.Name == "bootstrap_release" {
					t.Fatalf("failed verification reached a credential/code activation phase: %+v", phases)
				}
				cleanupMeasured = cleanupMeasured || phase.Name == "worker_cleanup"
			}
			if !cleanupMeasured {
				t.Fatalf("failure cleanup missing from timings: %+v", phases)
			}
			if released || len(p.issued) != 0 || len(p.restored) != 0 {
				t.Fatalf("unverified worker received work/credentials: released=%t issued=%v restored=%v", released, p.issued, p.restored)
			}
			if f.removed != 1 || f.networksRemoved != 1 {
				t.Fatal("failed verification left worker resources")
			}
		})
	}
}

func TestVerifiedBootstrapRestoresTokenOnlyAfterGuestProbe(t *testing.T) {
	f, m := newEngineFixture(t)
	p := &runtimeProxy{fakeProxy: fakeProxy{endpoint: "http://auth:18080"}}
	m.proxy = p
	probe := m.execProbe
	checked := false
	m.execProbe = func(ctx context.Context, id string, args []string) ([]byte, error) {
		if args[1] == "probe" {
			if len(p.issued) != 0 || len(p.restored) != 0 {
				t.Fatal("credentials enabled before verification")
			}
			checked = true
		}
		if args[1] == "release" && (!checked || len(p.restored) != 1) {
			t.Fatal("command released before verification/token restoration")
		}
		return probe(ctx, id, args)
	}
	id, err := m.Run(RunOptions{Name: "worker", ProjectDir: t.TempDir(), Detach: true})
	if err != nil {
		t.Fatal(err)
	}
	if !checked || f.worker.Config.Entrypoint[0] != guestGuard {
		t.Fatal("trusted bootstrap bypassed")
	}
	if err = m.Remove(id, true); err != nil {
		t.Fatal(err)
	}
}

func TestRemovalCanResumeAfterDockerObjectDisappears(t *testing.T) {
	for _, failure := range []string{"network", "lost-response"} {
		t.Run(failure, func(t *testing.T) {
			f, m := newEngineFixture(t)
			p := &runtimeProxy{fakeProxy: fakeProxy{endpoint: "http://auth:18080"}}
			m.proxy = p
			id, err := m.Run(RunOptions{Name: "worker", ProjectDir: t.TempDir(), Detach: true, Task: "private task must not be journaled"})
			if err != nil {
				t.Fatal(err)
			}
			f.networkRemoveError = failure == "network"
			f.removeReplyLost = failure == "lost-response"
			if err = m.Remove(id, true); err == nil {
				t.Fatal("injected removal failure ignored")
			}
			if f.worker != nil {
				t.Fatal("fixture did not remove Docker object")
			}
			record, err := m.readCleanup(id)
			if err != nil {
				t.Fatal("cleanup record was lost:", err)
			}
			if _, ok := record.Labels[labelTask]; ok {
				t.Fatal("raw task leaked into recovery record")
			}
			f.networkRemoveError = false
			f.removeReplyLost = false
			recovered := &Manager{cli: m.cli, guard: m.guard, stateDir: m.stateDir, proxy: p}
			if err = recovered.RemoveByName("worker", true); err != nil {
				t.Fatal("new manager could not resume cleanup:", err)
			}
			if f.front != nil || f.networksRemoved != 1 {
				t.Fatal("network was not recovered")
			}
			if _, err = recovered.readCleanup(id); !os.IsNotExist(err) {
				t.Fatal("completed recovery record was retained", err)
			}
		})
	}
}

func TestCleanupRecordCannotBeUsedWithAnotherDaemon(t *testing.T) {
	f, m := newEngineFixture(t)
	id, err := m.Run(RunOptions{Name: "worker", ProjectDir: t.TempDir(), Detach: true})
	if err != nil {
		t.Fatal(err)
	}
	f.networkRemoveError = true
	if err = m.Remove(id, true); err == nil {
		t.Fatal("injection did not fail")
	}
	r, err := m.readCleanup(id)
	if err != nil {
		t.Fatal(err)
	}
	r.Endpoint = "unix:///another-daemon.sock"
	if err = m.writeCleanup(r); err != nil {
		t.Fatal(err)
	}
	if _, err = m.readCleanup(id); err == nil {
		t.Fatal("cross-daemon cleanup was accepted")
	}
}

func TestGuestIdentityAndResourcesRemainCheckedOnRestart(t *testing.T) {
	f, m := newEngineFixture(t)
	id, err := m.Run(RunOptions{Name: "worker", ProjectDir: t.TempDir(), Detach: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Stop(id, 1); err != nil {
		t.Fatal(err)
	}
	f.worker.HostConfig.Memory = 0
	if err = m.Start(id); err == nil || !strings.Contains(err.Error(), "resource") {
		t.Fatalf("unbounded restart accepted: %v", err)
	}
	if f.starts != 1 {
		t.Fatal("invalid restart dispatched work")
	}
}

func TestKataMountCannotForgeProbeOrOverwriteBootstrap(t *testing.T) {
	_, m := newEngineFixture(t)
	source := t.TempDir()
	for _, target := range []string{"/proc/1/status", "/sys/fs/cgroup/memory.max", "/dev", "/run", "/", guestGuard, "/run/codex-dock-control/release.sock"} {
		if err := m.protectKataMounts([]mount.Mount{{Source: source, Target: target}}); err == nil {
			t.Fatalf("trusted interface mount accepted: %s", target)
		}
	}
	if err := m.protectKataMounts([]mount.Mount{{Source: source, Target: "/fixtures"}}); err != nil {
		t.Fatal(err)
	}
}

type retryRevoker struct {
	runtimeProxy
	fail bool
}

func (p *retryRevoker) RevokeTokenChecked(name string) error {
	if p.fail {
		return fmt.Errorf("fixture revocation unavailable")
	}
	p.RevokeToken(name)
	return nil
}
func TestCleanupRetainsIntentWhenTokenRevocationFails(t *testing.T) {
	_, m := newEngineFixture(t)
	p := &retryRevoker{runtimeProxy: runtimeProxy{fakeProxy: fakeProxy{endpoint: "http://auth:18080"}}, fail: true}
	m.proxy = p
	id, err := m.Run(RunOptions{Name: "worker", ProjectDir: t.TempDir(), Detach: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Remove(id, true); err == nil {
		t.Fatal("revocation failure ignored")
	}
	if _, err := m.readCleanup(id); err != nil {
		t.Fatalf("intent was discarded: %v", err)
	}
	p.fail = false
	if err := m.RemoveByName("worker", true); err != nil {
		t.Fatal(err)
	}
	if _, err := m.readCleanup(id); !os.IsNotExist(err) {
		t.Fatalf("completed intent retained: %v", err)
	}
}
