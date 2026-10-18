package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/pacificbelt30/codex-dock/internal/diagnostics"
)

// CheckImage asks the trusted node to inspect a read-only image snapshot.
// No image program runs on the host and no worker/token is created.
func CheckImage(ctx context.Context, image, agent string) (diagnostics.Report, error) {
	var report diagnostics.Report
	c, err := NewClient()
	if err != nil {
		return report, err
	}
	defer func() { _ = c.Close() }()
	q := url.Values{"name": {image}, "agent": {agent}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker/images/check?"+q.Encode(), nil)
	if err != nil {
		return report, err
	}
	resp, err := c.HTTPClient().Do(req)
	if err != nil {
		return report, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var detail struct {
			Message string `json:"message"`
		}
		_ = json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&detail)
		return report, fmt.Errorf("image inspection failed (HTTP %d): %s; verify the image, node version and snapshotter storage", resp.StatusCode, detail.Message)
	}
	err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&report)
	return report, err
}
