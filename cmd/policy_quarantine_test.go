package cmd

import (
	"io"
	"strings"
	"testing"
)

func TestPolicyQuarantineRejectsMissingWorkerOrPolicyOverrides(t *testing.T) {
	for _, args := range [][]string{{"quarantine"}, {"revoke"}, {"quarantine", "--worker", "target", "--preset", "offline"}, {"quarantine", "--worker", "target", "--allow-domain", "example.com"}} {
		command := newPolicyCommand()
		command.SetOut(io.Discard)
		command.SetErr(io.Discard)
		command.SetArgs(args)
		if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "quarantine requires --worker") {
			t.Fatalf("invalid quarantine arguments accepted: %v %v", args, err)
		}
	}
}
