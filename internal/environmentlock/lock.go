// Package environmentlock pins the node image and hashes explicitly selected
// source files. Refreshing a lock is always an explicit host operation.
package environmentlock

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/pacificbelt30/codex-dock/internal/workerpolicy"
	"io"
	"os"
	"path/filepath"
)

type Lock struct {
	Version int               `json:"version"`
	Image   string            `json:"image"`
	ImageID string            `json:"image_id"`
	Sources map[string]string `json:"sources,omitempty"`
}

func New(root, image, id string, files []string) (Lock, error) {
	if id == "" {
		return Lock{}, fmt.Errorf("node image digest is unavailable; upgrade the node before locking")
	}
	l := Lock{Version: 1, Image: image, ImageID: id, Sources: map[string]string{}}
	if err := (workerpolicy.Environment{ImageID: id}).Validate(); err != nil {
		return l, err
	}
	for _, file := range files {
		if !filepath.IsLocal(file) {
			return l, fmt.Errorf("lock sources must be relative project files")
		}
		sum, err := hashFile(filepath.Join(root, file))
		if err != nil {
			return l, err
		}
		l.Sources[file] = sum
	}
	return l, nil
}
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("lock source must be a regular file")
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func Load(path, root, image string) (Lock, error) {
	f, err := os.Open(path)
	if err != nil {
		return Lock{}, err
	}
	defer func() { _ = f.Close() }()
	var l Lock
	decoder := json.NewDecoder(io.LimitReader(f, 1<<20))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&l); err != nil {
		return l, err
	}
	if l.Version != 1 || l.Image != image || l.ImageID == "" {
		return l, fmt.Errorf("environment lock version/image mismatch")
	}
	if err := (workerpolicy.Environment{ImageID: l.ImageID}).Validate(); err != nil {
		return l, err
	}
	for file, want := range l.Sources {
		if !filepath.IsLocal(file) {
			return l, fmt.Errorf("invalid lock source")
		}
		got, err := hashFile(filepath.Join(root, file))
		if err != nil {
			return l, err
		}
		if got != want {
			return l, fmt.Errorf("environment source changed: %s", file)
		}
	}
	return l, nil
}
func (l Lock) Write(path string) error {
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(append(data, '\n'))
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}
