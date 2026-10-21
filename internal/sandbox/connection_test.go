package sandbox

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/pacificbelt30/codex-dock/pkg/cxd/identity"
)

func TestWorkerConnectionUsesSavedGuestEndpointAndIdentity(t *testing.T) {
	f, m := newEngineFixture(t)
	m.proxy = &runtimeProxy{fakeProxy: fakeProxy{endpoint: "http://auth:18080", anthropic: true}}
	id, err := m.Run(RunOptions{Name: "connections", ProjectDir: t.TempDir(), Detach: true})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := m.Remove(id, true); err != nil {
			t.Error(err)
		}
	}()
	connections, err := m.WorkerConnections(context.Background(), id)
	if err != nil || len(connections) != 2 {
		t.Fatalf("connections: %+v %v", connections, err)
	}
	if connections[0].BaseURL != "http://172.25.0.2:18080/v1" || connections[0].Placeholder == "" || connections[0].ModelPolicyActive {
		t.Fatalf("lost endpoint or token: %+v", connections[0])
	}
	encoded, err := json.Marshal(identity.Identity{CaseID: "case", RunID: "run", ConnectionVersion: "connection", Provider: "openai"})
	if err != nil {
		t.Fatal(err)
	}
	f.worker.Config.Labels[labelPrefix+"token-identity"] = string(encoded)
	f.worker.Config.Labels[labelPrefix+"token-model-policy"] = `{"allowed_models":[]}`
	connections, err = m.WorkerConnections(context.Background(), id)
	if err != nil || len(connections) != 1 || connections[0].Provider != "openai" || !connections[0].ModelPolicyActive {
		t.Fatalf("identity/policy not reflected: %+v %v", connections, err)
	}
}

type transportPolicyProxy struct {
	fakeProxy
	active bool
}

func (p *transportPolicyProxy) ModelPolicyActive() bool { return p.active }
func TestHTTPTransportOptInPreservesUnrestrictedDefaults(t *testing.T) {
	for _, tc := range []struct{ global, token bool }{{}, {global: true}, {token: true}} {
		p := &transportPolicyProxy{fakeProxy: fakeProxy{endpoint: "http://auth:18080"}, active: tc.global}
		m := &Manager{proxy: p}
		opts := RunOptions{Agent: AgentCodex, pendingPlaceholder: "cdx-fixture"}
		if tc.token {
			opts.TokenModelPolicy = &identity.ModelPolicy{AllowedModels: []string{"cheap"}}
		}
		env, err := m.buildEnv("worker", opts, "")
		if err != nil {
			t.Fatal(err)
		}
		got, _ := envValue(env, "CODEX_DOCK_CODEX_HTTP")
		if (got == "1") != (tc.global || tc.token) {
			t.Fatalf("global=%v token=%v transport=%q", tc.global, tc.token, got)
		}
	}
}
