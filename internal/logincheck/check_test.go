package logincheck

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCredentialCheckUsesOfficialEndpointAndNoInference(t *testing.T) {
	for _, provider := range []string{"openai", "anthropic"} {
		client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodGet || r.URL.Scheme != "https" || r.URL.Path != "/v1/models" || r.Body != nil {
				t.Fatal("credential check changed its fixed read-only operation")
			}
			if provider == "openai" && (r.URL.Host != "api.openai.com" || r.Header.Get("Authorization") != "Bearer private-key") {
				t.Fatal("OpenAI check used an incorrect endpoint or credential")
			}
			if provider == "anthropic" && (r.URL.Host != "api.anthropic.com" || r.Header.Get("x-api-key") != "private-key" || r.Header.Get("anthropic-version") == "") {
				t.Fatal("Anthropic check used an incorrect endpoint or credential")
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":[]}`)), Header: make(http.Header)}, nil
		})}
		if err := check(context.Background(), client, provider, "private-key"); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCredentialCheckSanitizesFailures(t *testing.T) {
	for _, status := range []int{301, 401, 403, 429, 500} {
		client := &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader("private-reflected-key"))}, nil
		})}
		err := check(context.Background(), client, "openai", "private-key")
		if err == nil || strings.Contains(err.Error(), "private") {
			t.Fatal("credential check accepted failure or exposed a secret")
		}
	}
	client := &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("private-key")
	})}
	if err := check(context.Background(), client, "openai", "private-key"); err == nil || strings.Contains(err.Error(), "private") {
		t.Fatal("credential check exposed transport details")
	}
}
