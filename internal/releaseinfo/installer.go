package releaseinfo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Installer downloads the requested release's script and checks its published
// SHA-256 before returning a private, non-executable file. The checksum detects
// corruption/mismatches; it is not an independent publisher signature.
func Installer(ctx context.Context, tag, directory string) (string, error) {
	client := &http.Client{Timeout: 30 * time.Second, CheckRedirect: releaseRedirect}
	return installer(ctx, client, tag, directory)
}

func releaseRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 5 || req.URL.Scheme != "https" || req.URL.User != nil {
		return fmt.Errorf("invalid release redirect")
	}
	switch req.URL.Hostname() {
	case "github.com", "release-assets.githubusercontent.com", "objects.githubusercontent.com":
		return nil
	default:
		return fmt.Errorf("unrecognized release asset host")
	}
}

func installer(ctx context.Context, client *http.Client, tag, directory string) (string, error) {
	if !ValidTag(tag) {
		return "", fmt.Errorf("invalid release tag")
	}
	base := "https://github.com/" + Repository + "/releases/download/" + tag + "/"
	fetch := func(name string) ([]byte, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+name, nil)
		if err != nil {
			return nil, err
		}
		response, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("release asset download failed")
		}
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("release asset download failed (HTTP %d)", response.StatusCode)
		}
		data, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
		if err != nil || len(data) > 1<<20 {
			return nil, fmt.Errorf("release asset exceeds supported size")
		}
		return data, nil
	}
	checksums, err := fetch("SHA256SUMS")
	if err != nil {
		return "", err
	}
	var expected string
	for _, line := range strings.Split(string(checksums), "\n") {
		parts := strings.Fields(line)
		if len(parts) == 2 && parts[1] == "install.sh" {
			if expected != "" {
				return "", fmt.Errorf("duplicate installer checksum")
			}
			expected = parts[0]
		}
	}
	if decoded, err := hex.DecodeString(expected); err != nil || len(decoded) != sha256.Size {
		return "", fmt.Errorf("missing or invalid installer checksum")
	}
	data, err := fetch("install.sh")
	if err != nil {
		return "", err
	}
	actual := sha256.Sum256(data)
	if hex.EncodeToString(actual[:]) != strings.ToLower(expected) {
		return "", fmt.Errorf("release installer checksum mismatch")
	}
	path := filepath.Join(directory, "install.sh")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Write(data); err != nil {
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	return path, nil
}
