// Package releaseinfo reads Oaiso release metadata without authentication.
package releaseinfo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"
)

const Repository = "pacificbelt30/sandbox-oasis"

var releaseTag = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9.-]+)?$`)

type Release struct {
	Tag        string `json:"tag_name"`
	URL        string `json:"html_url"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}

func ValidTag(tag string) bool { return releaseTag.MatchString(tag) }

func Latest(ctx context.Context) (Release, error) {
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
		return fmt.Errorf("release metadata redirects are not accepted")
	}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/"+Repository+"/releases/latest", nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Oaiso-release-check")
	response, err := client.Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("could not reach GitHub Releases")
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("GitHub release check failed (HTTP %d)", response.StatusCode)
	}
	return decode(io.LimitReader(response.Body, (1<<20)+1))
}

func decode(reader io.Reader) (Release, error) {
	var value Release
	data, err := io.ReadAll(io.LimitReader(reader, (1<<20)+1))
	if err != nil || len(data) > 1<<20 || json.Unmarshal(data, &value) != nil || !ValidTag(value.Tag) || value.Draft || value.Prerelease || value.URL != "https://github.com/"+Repository+"/releases/tag/"+value.Tag {
		return Release{}, fmt.Errorf("invalid stable release metadata")
	}
	return value, nil
}
