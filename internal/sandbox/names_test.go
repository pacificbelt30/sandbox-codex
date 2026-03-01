package sandbox

import (
	"strings"
	"testing"
)

func TestGenerateName(t *testing.T) {
	seen := make(map[string]struct{})
	for i := 0; i < 100; i++ {
		name := generateName()
		if name == "" {
			t.Fatal("generateName returned empty string")
		}
		if !strings.HasPrefix(name, "codex-") {
			t.Errorf("name %q does not start with 'codex-'", name)
		}
		parts := strings.Split(name, "-")
		if len(parts) != 3 {
			t.Errorf("name %q does not match 'codex-<adj>-<noun>' format", name)
		}
		seen[name] = struct{}{}
	}
	// With 25 adjectives × 25 nouns = 625 combinations; 100 tries should
	// yield well more than 1 unique name.
	if len(seen) < 5 {
		t.Errorf("generateName lacks entropy: only %d unique names in 100 calls", len(seen))
	}
}

func TestAdjectivesNounsNonEmpty(t *testing.T) {
	if len(adjectives) == 0 {
		t.Error("adjectives list is empty")
	}
	if len(nouns) == 0 {
		t.Error("nouns list is empty")
	}
	for _, a := range adjectives {
		if a == "" {
			t.Error("empty adjective found")
		}
	}
	for _, n := range nouns {
		if n == "" {
			t.Error("empty noun found")
		}
	}
}
