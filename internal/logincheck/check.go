// Package logincheck performs explicit credential checks without inference.
package logincheck

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// APIKey checks the official models endpoint. It does not send a prompt, select
// a model, refresh OAuth or infer permission to use every model. Errors never
// contain a response body, request headers or the supplied credential.
func APIKey(ctx context.Context, provider, key string) error {
	return check(ctx, &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
		return fmt.Errorf("credential checks do not follow redirects")
	}}, provider, key)
}

func check(ctx context.Context, client *http.Client, provider, key string) error {
	if key == "" || strings.ContainsAny(key, "\r\n\x00") {
		return fmt.Errorf("invalid API key input")
	}
	endpoint := "https://api.openai.com/v1/models"
	if provider == "anthropic" {
		endpoint = "https://api.anthropic.com/v1/models?limit=1"
	} else if provider != "openai" {
		return fmt.Errorf("choose openai or anthropic")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("could not create credential check")
	}
	if provider == "openai" {
		req.Header.Set("Authorization", "Bearer "+key)
	} else {
		req.Header.Set("x-api-key", key)
		req.Header.Set("anthropic-version", "2023-06-01")
	}
	response, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("credential check could not reach the provider; no credentials were saved")
	}
	defer func() { _ = response.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("credential check failed (HTTP %d); check the key and its permissions", response.StatusCode)
	}
	return nil
}
