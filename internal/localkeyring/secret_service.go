// Package localkeyring uses an existing Linux Secret Service through its
// official secret-tool client. It never starts a service or stores a file key.
package localkeyring

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"time"
)

type SecretService struct{}

func (SecretService) run(ctx context.Context, input []byte, args ...string) ([]byte, error) {
	if os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" {
		return nil, fmt.Errorf("secret service needs an existing user D-Bus session; choose passphrase storage on headless hosts")
	}
	path, err := exec.LookPath("secret-tool")
	if err != nil {
		return nil, fmt.Errorf("secret service requires secret-tool (libsecret); no storage fallback was used")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Stdin = bytes.NewReader(input)
	var out limitedBuffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("secret service operation failed or timed out; unlock the existing user keyring and retry")
	}
	return out.Bytes(), nil
}

// Bound output even if an unrelated or malformed keyring item was returned.
type limitedBuffer struct{ bytes.Buffer }

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 4096 {
		return 0, fmt.Errorf("keyring output exceeds limit")
	}
	return b.Buffer.Write(p)
}
func (s SecretService) Put(ctx context.Context, id string, key []byte) error {
	encoded := []byte(base64.StdEncoding.EncodeToString(key))
	defer clear(encoded)
	_, err := s.run(ctx, encoded, "store", "--label=Oaiso credential wrapping key", "application", "oaiso", "key-id", id)
	return err
}
func (s SecretService) Get(ctx context.Context, id string) ([]byte, error) {
	data, err := s.run(ctx, nil, "lookup", "application", "oaiso", "key-id", id)
	defer clear(data)
	if err != nil {
		return nil, err
	}
	key, err := base64.StdEncoding.DecodeString(string(bytes.TrimSpace(data)))
	if err != nil || len(key) != 32 {
		clear(key)
		return nil, fmt.Errorf("invalid or missing Oaiso keyring key")
	}
	return key, nil
}
func (s SecretService) Delete(ctx context.Context, id string) error {
	_, err := s.run(ctx, nil, "clear", "application", "oaiso", "key-id", id)
	return err
}
