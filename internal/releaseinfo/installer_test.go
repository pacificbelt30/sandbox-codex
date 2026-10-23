package releaseinfo

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestInstallerRequiresMatchingUniqueChecksum(t *testing.T) {
	script := "#!/bin/bash\nexit 0\n"
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(script)))
	for _, checksums := range []string{digest + "  install.sh\n", strings.Repeat("0", 64) + "  install.sh\n", digest + "  install.sh\n" + digest + "  install.sh\n", digest + "  unrelated.sh\n"} {
		t.Run(fmt.Sprint(len(checksums), checksums[:8]), func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Scheme != "https" || r.URL.Host != "github.com" || !strings.HasPrefix(r.URL.Path, "/"+Repository+"/releases/download/v0.0.2/") || r.Method != http.MethodGet || r.Header.Get("Authorization") != "" {
					t.Fatal("installer download used an unexpected endpoint or authentication")
				}
				body := script
				if strings.HasSuffix(r.URL.Path, "SHA256SUMS") {
					body = checksums
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
			})}
			dir := t.TempDir()
			path, err := installer(context.Background(), client, "v0.0.2", dir)
			if checksums == digest+"  install.sh\n" {
				if err != nil {
					t.Fatal(err)
				}
				data, _ := os.ReadFile(path)
				info, _ := os.Stat(path)
				if string(data) != script || info.Mode().Perm() != 0600 {
					t.Fatal("verified installer content or permissions differ")
				}
			} else if err == nil || path != "" {
				t.Fatal("unverified installer was made available for execution")
			}
		})
	}
}

func TestReleaseRedirectRejectsDowngradeAndUnknownHosts(t *testing.T) {
	for _, target := range []string{"http://github.com/file", "https://unexpected.example/file", "https://user:secret@github.com/file"} {
		parsed, _ := url.Parse(target)
		if err := releaseRedirect(&http.Request{URL: parsed}, nil); err == nil {
			t.Fatal("unsafe release redirect was accepted")
		}
	}
	parsed, _ := url.Parse("https://release-assets.githubusercontent.com/file")
	if err := releaseRedirect(&http.Request{URL: parsed}, nil); err != nil {
		t.Fatal(err)
	}
}
