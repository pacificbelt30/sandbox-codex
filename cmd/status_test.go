package cmd

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/pacificbelt30/codex-dock/internal/diagnostics"
	"github.com/pacificbelt30/codex-dock/pkg/cxd/credentialstore"
)

func TestProfileStatusSeparatesStoredFromConnectionAndInference(t *testing.T) {
	isolatedProfiles(t)
	authProfileName = "work"
	store, _ := openAuthProfile("work")
	if err := store.Create(context.Background(), "work", credentialstore.Credentials{OpenAIAPIKey: "fixture-secret-should-not-be-displayed"}); err != nil {
		t.Fatal(err)
	}
	report := diagnostics.Report{}
	profileStatus(context.Background(), &report, false)
	data, _ := json.Marshal(report)
	if strings.Contains(string(data), "fixture-secret") || !strings.Contains(string(data), `"status":"unchecked"`) || !strings.Contains(string(data), "not tested") || report.Err() != nil {
		t.Fatal("stored credentials were disclosed or treated as a successful live connection")
	}
}
