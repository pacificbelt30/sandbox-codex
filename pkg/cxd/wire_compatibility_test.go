package cxd

import (
	"encoding/json"
	"testing"
)

// Existing request IDs must keep the exact pre-addition fingerprint input when
// all new policies are unspecified. This literal comes from commit 0c66e85.
func TestZeroAdditionsPreserveLaunchWireFormat(t *testing.T) {
	data, err := json.Marshal(LaunchRequest{RequestID: "request-1", ProjectDir: "/workspace", Image: "worker"})
	if err != nil {
		t.Fatal(err)
	}
	const legacy = `{"RequestID":"request-1","ProjectDir":"/workspace","Image":"worker","Profile":"","Runtime":"","Agent":"","AgentArgs":null,"Model":"","ApprovalMode":"","Branch":"","UseWorktree":false,"NewBranch":false,"ReadOnly":false,"ReadOnlyRootFS":false,"NoGitMetadata":false,"NoInternet":false,"EgressAllowDomains":null,"WatchPolling":false,"User":"","Mounts":null,"TokenTTL":0,"Identity":{"case_id":"","run_id":"","connection_version":"","provider":""}}`
	if string(data) != legacy {
		t.Fatalf("zero-valued additions changed launch identity:\n%s", data)
	}
}
