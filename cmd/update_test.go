package cmd

import "testing"

// TestExecuteUpdate_GoNotFound verifies that executeUpdate fails fast with a
// clear error when the "go" binary isn't on PATH.
func TestExecuteUpdate_GoNotFound(t *testing.T) {
	t.Setenv("PATH", "")

	err := executeUpdate(t.Context())
	if err == nil {
		t.Fatal("expected error when go is not found in PATH")
	}
}
