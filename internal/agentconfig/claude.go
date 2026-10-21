package agentconfig

import (
	"fmt"
	"net/url"
	"strings"
)

const (
	ClaudeAPIKeyEnv = "ANTHROPIC_API_KEY"
	ClaudeOAuthEnv  = "CLAUDE_CODE_OAUTH_TOKEN"
)

// ClaudeEnv selects the official CLI's API-key or subscription path. All
// credentials here are worker placeholders; real credentials stay in the proxy.
// Clear competing selectors, including those inherited by Exec from the image.
func ClaudeEnv(endpoint, placeholder string, oauth bool) ([]string, error) {
	u, err := url.Parse(endpoint)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.ContainsAny(endpoint, "\x00\r\n") {
		return nil, fmt.Errorf("invalid worker proxy endpoint")
	}
	apiKey, accessToken := placeholder, ""
	if oauth {
		apiKey, accessToken = "", placeholder
	}
	return []string{
		"ANTHROPIC_BASE_URL=" + endpoint,
		ClaudeAPIKeyEnv + "=" + apiKey,
		ClaudeOAuthEnv + "=" + accessToken,
		"ANTHROPIC_AUTH_TOKEN=", "ANTHROPIC_PROFILE=",
		"CLAUDE_CODE_OAUTH_REFRESH_TOKEN=", "CLAUDE_CODE_OAUTH_SCOPES=",
		"CLAUDE_CODE_USE_BEDROCK=", "CLAUDE_CODE_USE_VERTEX=", "CLAUDE_CODE_USE_FOUNDRY=",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1",
	}, nil
}
