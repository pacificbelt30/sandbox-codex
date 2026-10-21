package cxd

import (
	"context"
	"fmt"

	"github.com/pacificbelt30/codex-dock/internal/agentconfig"
)

type Transport string

const (
	TransportHTTP      Transport = "http"
	TransportSSE       Transport = "sse"
	TransportWebSocket Transport = "websocket"
)

// ConnectionInfo is safe to persist as metadata. It contains no token values.
// Endpoints are reachable from the worker network, not necessarily the host.
type ConnectionInfo struct {
	Provider          string
	BaseURL           string
	AuthHeader        string
	AuthScheme        string
	TokenEnvironment  string
	Transports        []Transport
	ModelPolicyActive bool
}

// Connections reports the saved guest endpoint and current proxy constraints.
// This does not issue or activate a token; Create still requires Start.
func (c *Client) Connections(ctx context.Context, worker string) ([]ConnectionInfo, error) {
	connections, err := c.manager.WorkerConnections(ctx, worker)
	if err != nil {
		return nil, classify(err)
	}
	result := make([]ConnectionInfo, 0, len(connections))
	for _, v := range connections {
		info := ConnectionInfo{Provider: v.Provider, BaseURL: v.BaseURL, AuthHeader: "Authorization", AuthScheme: "Bearer", TokenEnvironment: agentconfig.CodexTokenEnv, Transports: []Transport{TransportHTTP, TransportSSE}, ModelPolicyActive: v.ModelPolicyActive}
		if v.Provider == "openai" && !v.ModelPolicyActive {
			info.Transports = append(info.Transports, TransportWebSocket)
		}
		if v.Provider == "anthropic" {
			info.AuthHeader = "X-Api-Key"
			info.AuthScheme = ""
			info.TokenEnvironment = v.TokenEnvironment
			if v.TokenEnvironment == agentconfig.ClaudeOAuthEnv {
				info.AuthHeader, info.AuthScheme = "Authorization", "Bearer"
			}
		}
		result = append(result, info)
	}
	return result, nil
}

// AgentCommand contains a short-lived placeholder in Env. Do not log Env.
// Append native agent arguments to Argv. Connection settings work independently
// of generated user configuration.
type AgentCommand struct {
	Argv []string
	Env  []string `json:"-"`
}

// ClaudeCommand selects the worker's saved API-key or subscription connection.
// User/project settings are disabled for a reproducible invocation; callers can
// append explicit CLI options. Neither argv nor Env contains host credentials.
func (c *Client) ClaudeCommand(ctx context.Context, worker string) (AgentCommand, error) {
	connections, err := c.manager.WorkerConnections(ctx, worker)
	if err != nil {
		return AgentCommand{}, classify(err)
	}
	for _, v := range connections {
		if v.Provider != "anthropic" {
			continue
		}
		env, err := agentconfig.ClaudeEnv(v.BaseURL, v.Placeholder, v.TokenEnvironment == agentconfig.ClaudeOAuthEnv)
		if err != nil {
			return AgentCommand{}, err
		}
		return AgentCommand{Argv: []string{"claude", "--setting-sources", ""}, Env: env}, nil
	}
	return AgentCommand{}, fmt.Errorf("worker has no Anthropic proxy connection")
}

// CodexHTTPCommand explicitly selects HTTP/SSE for this invocation. Other
// launches retain their transport defaults unless model policy requires HTTP.
func (c *Client) CodexHTTPCommand(ctx context.Context, worker string) (AgentCommand, error) {
	connections, err := c.manager.WorkerConnections(ctx, worker)
	if err != nil {
		return AgentCommand{}, classify(err)
	}
	for _, v := range connections {
		if v.Provider != "openai" {
			continue
		}
		args, err := agentconfig.CodexHTTPArgs(v.BaseURL)
		if err != nil {
			return AgentCommand{}, err
		}
		return AgentCommand{Argv: args, Env: []string{agentconfig.CodexTokenEnv + "=" + v.Placeholder}}, nil
	}
	return AgentCommand{}, fmt.Errorf("worker has no OpenAI proxy connection")
}
