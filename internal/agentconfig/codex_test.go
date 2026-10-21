package agentconfig

import (
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestCodexOverridesAreIndependentValidTOML(t *testing.T) {
	args, err := CodexHTTPArgs("http://10.253.1.2:18080/v1")
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for i := 1; i < len(args); i += 2 {
		if args[i] != "-c" {
			t.Fatalf("unexpected argument: %s", args[i])
		}
		lines = append(lines, args[i+1])
	}
	var config struct {
		Provider  string `toml:"model_provider"`
		Providers map[string]struct {
			BaseURL    string `toml:"base_url"`
			EnvKey     string `toml:"env_key"`
			WebSockets bool   `toml:"supports_websockets"`
			OpenAIAuth bool   `toml:"requires_openai_auth"`
		} `toml:"model_providers"`
	}
	if err := toml.Unmarshal([]byte(strings.Join(lines, "\n")), &config); err != nil {
		t.Fatal(err)
	}
	p := config.Providers[config.Provider]
	if config.Provider != "cxd_proxy" || p.BaseURL != "http://10.253.1.2:18080/v1" || p.EnvKey != CodexTokenEnv || p.WebSockets || p.OpenAIAuth {
		t.Fatalf("wrong transport config: %+v", config)
	}
	for _, endpoint := range []string{"ftp://proxy/v1", "http://secret@proxy/v1", "http://proxy/v1?key=secret", "http://proxy/v1#fragment", "http://proxy/\n"} {
		if _, err := CodexHTTPArgs(endpoint); err == nil {
			t.Fatalf("accepted unsafe endpoint: %q", endpoint)
		}
	}
}
