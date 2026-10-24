package localkeyring

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSecretServiceUnavailableDoesNotFallback(t *testing.T) {
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "")
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	_, err := (SecretService{}).Get(context.Background(), "public-key-id")
	if err == nil || !strings.Contains(err.Error(), "existing user D-Bus") {
		t.Fatal("missing service did not fail closed")
	}
}
func TestSecretToolUsesSecretStdinAndBoundedOutput(t *testing.T) {
	dir := t.TempDir()
	tool := filepath.Join(dir, "secret-tool")
	// The fake client validates that only public lookup attributes are in argv;
	// real Secret Service interoperability still requires a desktop integration test.
	script := `#!/bin/sh
case "$*" in
 'store --label=Oaiso credential wrapping key application oaiso key-id public-id')
  value=$(cat)
  test "$value" = 'AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=' || exit 2 ;;
 'lookup application oaiso key-id public-id') printf 'AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=' ;;
 'clear application oaiso key-id public-id') exit 0 ;;
 *) exit 3 ;;
esac
`
	if err := os.WriteFile(tool, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path=/fake/user-bus")
	ring := SecretService{}
	if err := ring.Put(context.Background(), "public-id", make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	key, err := ring.Get(context.Background(), "public-id")
	if err != nil || len(key) != 32 {
		t.Fatal("keyring output failed", err)
	}
	if err = ring.Delete(context.Background(), "public-id"); err != nil {
		t.Fatal(err)
	}
}
