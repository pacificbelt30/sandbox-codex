package onboarding

import (
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestSelectingProfilePreservesUnrelatedSettingsAndComments(t *testing.T) {
	for _, original := range []string{
		"",
		"# keep this comment\nauth_profile = 'old' # keep this too\n[proxy] # keep header\nforward_default_deny = true\nforward_allow_domains = ['=git.example']\n[run]\nagent = 'shell'\n",
		"# keep\n[proxy]",
		"auth_profile = '''old'''\r\n[proxy]\r\nadmin_secret_file = '/existing/secret'\r\n",
		"'auth_profile' = 'old'\n[\"proxy\"] # comment\nadmin_secret = 'existing-secret'\n",
	} {
		updated, err := SelectProfile([]byte(original), "new", "/private/admin-secret")
		if err != nil {
			t.Fatalf("%q: %v", original, err)
		}
		var before, after map[string]any
		_ = toml.Unmarshal([]byte(original), &before)
		if err := toml.Unmarshal(updated, &after); err != nil || after["auth_profile"] != "new" {
			t.Fatal("profile selection did not produce valid global configuration")
		}
		if strings.Contains(original, "# keep") && !strings.Contains(string(updated), "# keep") {
			t.Fatal("profile selection removed a comment")
		}
		if strings.Contains(original, "forward_allow_domains") && !strings.Contains(string(updated), "forward_allow_domains = ['=git.example']") {
			t.Fatal("profile selection changed the egress policy")
		}
		if strings.Contains(original, "existing-secret") && !strings.Contains(string(updated), "admin_secret = 'existing-secret'") {
			t.Fatal("profile selection replaced existing admin authority")
		}
		if strings.Contains(original, "/existing/secret") && !strings.Contains(string(updated), "/existing/secret") {
			t.Fatal("profile selection replaced an existing secret file")
		}
	}
}

func TestSelectingProfileRejectsUnsafeOrUnsupportedConfiguration(t *testing.T) {
	for _, original := range []string{"auth_profile = 123", "[proxy\n", "proxy = 'invalid'", "proxy = {forward_default_deny = true}"} {
		if _, err := SelectProfile([]byte(original), "new", "/private/secret"); err == nil {
			t.Fatalf("accepted configuration that could not be safely edited: %q", original)
		}
	}
}
