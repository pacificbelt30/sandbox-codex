package authproxy

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// workerRequest authenticates before any upstream dial or credential injection.
// A lease also bounds WebSocket connections, which can carry multiple turns.
func (p *Proxy) workerRequest(r *http.Request, allowAPIKey bool) (*http.Request, func(), bool) {
	var token string
	values := r.Header.Values("Authorization")
	if len(values) > 1 {
		return nil, nil, false
	}
	if len(values) == 1 {
		fields := strings.Fields(values[0])
		if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") {
			return nil, nil, false
		}
		token = fields[1]
	}
	if allowAPIKey {
		keys := r.Header.Values("X-Api-Key")
		if len(keys) > 1 {
			return nil, nil, false
		}
		if len(keys) == 1 {
			if keys[0] == "" || token != "" && token != keys[0] {
				return nil, nil, false
			}
			token = keys[0]
		}
	}
	if token == "" {
		return nil, nil, false
	}
	p.mu.RLock()
	var lease context.Context
	for _, rec := range p.tokens {
		if rec.Token == token && time.Now().Before(rec.ExpiresAt) && rec.context != nil && rec.context.Err() == nil {
			if !p.brokerProviderAllowed(rec.Identity.Provider, r.URL.Path) {
				break
			}
			lease = rec.context
			break
		}
	}
	p.mu.RUnlock()
	if lease == nil {
		return nil, nil, false
	}
	ctx, cancel := context.WithCancel(r.Context())
	stop := context.AfterFunc(lease, cancel)
	return r.WithContext(ctx), func() { stop(); cancel() }, true
}

func providerRouteAllowed(provider, path string) bool {
	switch provider {
	case "", "any":
		return true
	case "openai":
		return strings.HasPrefix(path, "/v1/") || strings.HasPrefix(path, "/chatgpt/")
	case "anthropic":
		return strings.HasPrefix(path, "/anthropic/")
	default:
		return false
	}
}
