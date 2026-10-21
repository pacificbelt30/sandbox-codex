// Package agentconfig builds explicit client overrides independent of user files.
package agentconfig

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const CodexTokenEnv = "CXD_PROXY_TOKEN"

// CodexHTTPArgs is tested with the worker image's pinned Codex CLI. No host
// credential is included in argv: env_key refers to a short-lived placeholder.
func CodexHTTPArgs(endpoint string) ([]string, error) {
	u, err := url.Parse(endpoint)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.ContainsAny(endpoint, "\x00\r\n") {
		return nil, fmt.Errorf("invalid worker proxy endpoint")
	}
	provider := `{name="cxd_proxy",base_url=` + strconv.Quote(endpoint) + `,env_key="` + CodexTokenEnv + `",wire_api="responses",requires_openai_auth=false,supports_websockets=false}`
	return []string{"codex", "-c", `model_provider="cxd_proxy"`, "-c", "model_providers.cxd_proxy=" + provider,
		"-c", "features.apps=false",
		"-c", "features.enable_request_compression=false",
		"-c", "features.responses_websockets=false", "-c", "features.responses_websockets_v2=false"}, nil
}
