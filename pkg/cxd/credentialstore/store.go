// Package credentialstore defines a host-side credential adapter for isolated
// cxd auth proxies. Real credentials never travel through a worker VM.
package credentialstore

import (
	"context"
	"errors"
)

var ErrConflict = errors.New("credential connection changed")
var ErrNotFound = errors.New("credential connection not found")

// Credentials is one case's current credential generation. A provider must
// use either its API key or its OAuth record, never both.
type Credentials struct {
	Version           string          `json:"version"`
	ConnectionVersion string          `json:"connection_version"`
	OpenAIAPIKey      string          `json:"openai_api_key,omitempty"`
	OpenAIOAuth       *OpenAIOAuth    `json:"openai_oauth,omitempty"`
	AnthropicAPIKey   string          `json:"anthropic_api_key,omitempty"`
	AnthropicOAuth    *AnthropicOAuth `json:"anthropic_oauth,omitempty"`
}

type OpenAIOAuth struct {
	IDToken      string `json:"id_token"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	AccountID    string `json:"account_id"`
	LastRefresh  string `json:"last_refresh"`
	ExpiresAt    int64  `json:"expires_at"`
	TokenType    string `json:"token_type"`
}

type AnthropicOAuth struct {
	AccessToken      string   `json:"access_token"`
	RefreshToken     string   `json:"refresh_token"`
	ExpiresAt        int64    `json:"expires_at"`
	SubscriptionType string   `json:"subscription_type"`
	Scopes           []string `json:"scopes"`
}

// Store is implemented by the host product. SaveRefresh must compare version
// atomically with its durable source of truth and return the new version only
// after the refreshed credential is committed. A changed or revoked case must
// return ErrConflict or ErrNotFound. Never log the credential arguments.
type Store interface {
	Load(context.Context, string) (Credentials, error)
	SaveRefresh(context.Context, string, string, string, Credentials) (string, error)
}
