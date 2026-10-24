package localkeyring

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestBusDiscoveryUsesOnlyPrivateOwnedSocketDirectory(t *testing.T) {
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "")
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", filepath.Join(dir, "bus"))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if address, err := BusAddress(); err != nil || address != "unix:path="+filepath.Join(dir, "bus") {
		t.Fatal("private bus not discovered", err)
	}
	if err = os.Chmod(dir, 0770); err != nil {
		t.Fatal(err)
	}
	if _, err = BusAddress(); err == nil {
		t.Fatal("shared runtime directory accepted")
	}
}

func TestPrivatePathsIncludeDecodedBusAndKeyringFiles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", "/private/xdg-data")
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path=/private/bus%20socket,guid=public")
	paths, err := PrivatePaths()
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(paths, "\n")
	if !strings.Contains(joined, "/private/bus socket") || !strings.Contains(joined, "/private/xdg-data/keyrings") {
		t.Fatal("keyring authority not protected")
	}
}

func TestHeadlessSessionLockRejectsParallelSessionAndSymlink(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "keyring")
	lock, err := SessionFileLock(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = SessionFileLock(dir); err == nil {
		t.Fatal("two services can overwrite the same keyring")
	}
	_ = lock.Close()
	if err = os.Remove(filepath.Join(dir, ".session.lock")); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "foreign")
	if err = os.WriteFile(target, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(target, filepath.Join(dir, ".session.lock")); err != nil {
		t.Fatal(err)
	}
	if _, err = SessionFileLock(dir); err == nil {
		t.Fatal("symlink lock adopted")
	}
}

func TestProbeNeverFallsBackOnUnavailableBus(t *testing.T) {
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "")
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	if err := (SecretService{}).Probe(context.Background()); err == nil {
		t.Fatal("unavailable keyring accepted")
	}
}

func TestSessionCancellationReapsOwnedKeyring(t *testing.T) {
	if _, err := exec.LookPath("dbus-daemon"); err != nil {
		t.Skip("dbus-daemon required for lifecycle test")
	}
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "daemon-pid")
	item := filepath.Join(dir, "probe-item")
	t.Setenv("KEYRING_TEST_PID", pidFile)
	t.Setenv("KEYRING_TEST_ITEM", item)
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	fakeDaemon := `#!/bin/sh
cat > /dev/null
printf '%s' "$$" > "$KEYRING_TEST_PID"
exec sleep 300
`
	fakeTool := `#!/bin/sh
case "$1" in
 store) cat > "$KEYRING_TEST_ITEM" ;;
 lookup) cat "$KEYRING_TEST_ITEM" ;;
 clear) rm "$KEYRING_TEST_ITEM" ;;
 *) exit 2 ;;
esac
`
	for name, body := range map[string]string{"gnome-keyring-daemon": fakeDaemon, "secret-tool": fakeTool} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	command := exec.CommandContext(ctx, "sleep", "300")
	done := make(chan error, 1)
	go func() {
		done <- Session(ctx, filepath.Join(dir, "data"), []byte("fixture lifecycle passphrase"), command)
	}()
	var pid int
	for i := 0; i < 200; i++ {
		if data, err := os.ReadFile(pidFile); err == nil {
			_, _ = fmt.Sscan(string(data), &pid)
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if pid == 0 {
		cancel()
		<-done
		t.Fatal("owned keyring did not start")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("canceled keyring session did not return")
	}
	if err := syscall.Kill(pid, 0); err == nil {
		t.Fatal("owned keyring process was not reaped")
	}
}
