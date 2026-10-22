package authproxy

import (
	"encoding/base64"
	"encoding/json"
	"strings"
)

// workerIDToken is CLI-readable metadata, never an issuer credential. Codex
// 0.163.0 requires a three-part JWT in auth.json but only decodes its payload.
// Build a fresh unsigned envelope with an invalid signature, copying neither
// the issuer's header/signature nor arbitrary claims (including email/user IDs).
// The proxy, not these claims, authenticates upstream requests.
func workerIDToken(creds OAuthCredentials) string {
	type accountClaims struct {
		AccountID string `json:"chatgpt_account_id,omitempty"`
		Plan      string `json:"chatgpt_plan_type,omitempty"`
		FedRAMP   bool   `json:"chatgpt_account_is_fedramp,omitempty"`
	}
	var claims struct {
		Auth accountClaims `json:"https://api.openai.com/auth"`
	}
	parts := strings.Split(creds.IDToken, ".")
	if len(parts) == 3 {
		if payload, err := base64.RawURLEncoding.DecodeString(parts[1]); err == nil {
			if err := json.Unmarshal(payload, &claims); err != nil {
				claims.Auth = accountClaims{}
			}
		}
	}
	if creds.AccountID != "" {
		claims.Auth.AccountID = creds.AccountID
	}
	// Only string/bool fields are marshalled, so encoding cannot fail.
	payload, _ := json.Marshal(claims)
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	return header + "." + base64.RawURLEncoding.EncodeToString(payload) + ".cxd-placeholder"
}

// Preserve only fixed codes used by the pinned CLI's permanent/transient
// refresh classification. Issuer bodies/messages can reflect real credentials.
func workerRefreshErrorCode(body []byte) string {
	var response struct {
		Error json.RawMessage `json:"error"`
		Code  string          `json:"code"`
	}
	if json.Unmarshal(body, &response) != nil {
		return "oauth_refresh_failed"
	}
	code := response.Code
	var text string
	var nested struct {
		Code string `json:"code"`
	}
	if json.Unmarshal(response.Error, &text) == nil && strings.TrimSpace(text) != "" {
		code = text
	} else if json.Unmarshal(response.Error, &nested) == nil && strings.TrimSpace(nested.Code) != "" {
		code = nested.Code
	}
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "refresh_token_expired", "refresh_token_reused", "refresh_token_invalidated", "invalid_grant":
		return strings.ToLower(strings.TrimSpace(code))
	default:
		return "oauth_refresh_failed"
	}
}
