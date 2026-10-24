package localkeyring

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/pacificbelt30/codex-dock/pkg/cxd/credentialstore"
)

// DataDirectory is private storage for headless sessions, separate from the
// desktop login keyring. Secret Service stores only encrypted keyring files here.
func DataDirectory() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "codex-dock", "keyring"), nil
}

// Probe checks storage and read-back before collecting a provider credential.
// It never reads other items, and the random probe value is discarded.
func (s SecretService) Probe(ctx context.Context) error {
	var nonce [32]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return err
	}
	defer clear(nonce[:])
	id := "probe-" + hex.EncodeToString(nonce[:16])
	if err := s.Put(ctx, id, nonce[:]); err != nil {
		return err
	}
	saved, err := s.Get(ctx, id)
	defer clear(saved)
	cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	removeErr := s.Delete(cleanup, id)
	if err != nil || !bytes.Equal(saved, nonce[:]) {
		return fmt.Errorf("keyring read-back check failed")
	}
	return removeErr
}

// Session owns a private foreground D-Bus and keyring. Both are stopped and
// reaped when the command ends or is canceled. No daemon is adopted/replaced.
func Session(ctx context.Context, dataDir string, pass []byte, command *exec.Cmd) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("keyring sessions require Linux")
	}
	if len(pass) < 12 || len(pass) > 4096 {
		return fmt.Errorf("keyring passphrase must contain 12..4096 bytes")
	}
	if _, err := credentialstore.NewFileStore(dataDir); err != nil {
		return err
	}
	address, stopBus, err := startSessionBus(ctx)
	if err != nil {
		return err
	}
	defer stopBus()
	control, err := os.MkdirTemp("", "oaiso-keyring-control-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(control) }()
	daemonCtx, stop := context.WithCancel(ctx)
	defer stop()
	daemon := exec.CommandContext(daemonCtx, "gnome-keyring-daemon", "--foreground", "--components=secrets", "--control-directory="+control, "--unlock")
	daemon.Env = append(cleanSessionEnvironment(), "XDG_DATA_HOME="+dataDir, "DBUS_SESSION_BUS_ADDRESS="+address)
	input, writer, err := os.Pipe()
	if err != nil {
		return err
	}
	defer func() { _ = input.Close(); _ = writer.Close() }()
	daemon.Stdin = input
	// Daemon diagnostics can contain provider/keyring state; never forward them.
	if err = daemon.Start(); err != nil {
		return fmt.Errorf("starting keyring failed; install gnome-keyring")
	}
	done := make(chan struct{})
	go func() { _ = daemon.Wait(); close(done) }()
	defer func() {
		_ = daemon.Process.Signal(syscall.SIGTERM)
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			stop()
			<-done
		}
	}()
	_ = input.Close()
	if _, err = writer.Write(pass); err != nil {
		return fmt.Errorf("keyring secret input failed")
	}
	_ = writer.Close()
	clear(pass)
	ready, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	for {
		if err = (SecretService{Address: address}).Probe(ready); err == nil {
			break
		}
		select {
		case <-done:
			return fmt.Errorf("keyring service exited before unlocking")
		case <-ready.Done():
			return fmt.Errorf("keyring could not be unlocked; check its passphrase (no storage fallback)")
		case <-time.After(100 * time.Millisecond):
		}
	}
	if command.Env == nil {
		command.Env = os.Environ()
	}
	command.Env = append(command.Env, "DBUS_SESSION_BUS_ADDRESS="+address)
	return command.Run()
}

func cleanSessionEnvironment() []string {
	env := os.Environ()
	for _, key := range []string{"XDG_DATA_HOME", "GNOME_KEYRING_CONTROL", "GNOME_KEYRING_PID"} {
		for i := len(env) - 1; i >= 0; i-- {
			if len(env[i]) > len(key) && env[i][:len(key)+1] == key+"=" {
				env = append(env[:i], env[i+1:]...)
			}
		}
	}
	return env
}

// SessionFileLock prevents two headless services from opening the same files.
func SessionFileLock(dataDir string) (*os.File, error) {
	if _, err := credentialstore.NewFileStore(dataDir); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dataDir, ".session.lock"), os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) || stat.Nlink != 1 || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		_ = f.Close()
		return nil, fmt.Errorf("unsafe keyring lock file")
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("a headless keyring session is already active")
	}
	return f, nil
}

// BusAddress discovers an existing user bus without starting/replacing it.
func BusAddress() (string, error) {
	if address := os.Getenv("DBUS_SESSION_BUS_ADDRESS"); address != "" {
		return address, nil
	}
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = fmt.Sprintf("/run/user/%d", os.Geteuid())
	}
	info, err := os.Lstat(dir)
	if err == nil {
		stat, ok := info.Sys().(*syscall.Stat_t)
		if ok && stat.Uid == uint32(os.Geteuid()) && info.IsDir() && info.Mode().Perm() == 0700 {
			socket := filepath.Join(dir, "bus")
			if f, e := os.Lstat(socket); e == nil && f.Mode()&os.ModeSocket != 0 {
				stat, ok := f.Sys().(*syscall.Stat_t)
				if ok && stat.Uid == uint32(os.Geteuid()) {
					return "unix:path=" + socket, nil
				}
			}
		}
	}
	return "", fmt.Errorf("secret service needs an existing user D-Bus session; run oaiso auth keyring session")
}

// PrivatePaths must never be shared with a worker, including the service socket
// that can release a profile's decryption key to same-user clients.
func PrivatePaths() ([]string, error) {
	data, err := DataDirectory()
	if err != nil {
		return nil, err
	}
	paths := []string{data}
	xdg := os.Getenv("XDG_DATA_HOME")
	if xdg == "" {
		home, e := os.UserHomeDir()
		if e != nil {
			return nil, e
		}
		xdg = filepath.Join(home, ".local", "share")
	}
	paths = append(paths, filepath.Join(xdg, "keyrings"))
	address, err := BusAddress()
	if err != nil {
		return paths, nil
	}
	for _, entry := range strings.Split(address, ";") {
		if fields, ok := strings.CutPrefix(entry, "unix:"); ok {
			for _, field := range strings.Split(fields, ",") {
				if path, ok := strings.CutPrefix(field, "path="); ok {
					decoded, e := url.PathUnescape(path)
					if e != nil {
						return nil, fmt.Errorf("invalid D-Bus socket path")
					}
					paths = append(paths, decoded)
				}
			}
		}
	}
	return paths, nil
}

func startSessionBus(ctx context.Context) (string, func(), error) {
	dir, err := os.MkdirTemp("", "oaiso-keyring-bus-")
	if err != nil {
		return "", nil, err
	}
	cleanupDir := func() { _ = os.RemoveAll(dir) }
	var escaped bytes.Buffer
	_ = xml.EscapeText(&escaped, []byte(filepath.Join(dir, "bus")))
	config := `<busconfig><type>session</type><listen>unix:path=` + escaped.String() + `</listen><auth>EXTERNAL</auth><policy context="default"><allow send_destination="*"/><allow receive_sender="*"/><allow own="*"/></policy></busconfig>`
	path := filepath.Join(dir, "session.conf")
	if err = os.WriteFile(path, []byte(config), 0600); err != nil {
		cleanupDir()
		return "", nil, err
	}
	busCtx, cancel := context.WithCancel(ctx)
	bus := exec.CommandContext(busCtx, "dbus-daemon", "--nofork", "--config-file="+path, "--print-address=1")
	output, err := bus.StdoutPipe()
	if err != nil {
		cancel()
		cleanupDir()
		return "", nil, err
	}
	if err = bus.Start(); err != nil {
		cancel()
		cleanupDir()
		return "", nil, fmt.Errorf("starting private D-Bus failed; install dbus-daemon")
	}
	done := make(chan struct{})
	go func() { _ = bus.Wait(); close(done) }()
	cleanup := func() { cancel(); <-done; cleanupDir() }
	type result struct {
		address string
		err     error
	}
	ready := make(chan result, 1)
	go func() {
		address, e := bufio.NewReader(io.LimitReader(output, 1024)).ReadString('\n')
		ready <- result{strings.TrimSpace(address), e}
	}()
	select {
	case r := <-ready:
		if r.err == nil && strings.HasPrefix(r.address, "unix:path="+filepath.Join(dir, "bus")) {
			return r.address, cleanup, nil
		}
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
	}
	cleanup()
	return "", nil, fmt.Errorf("private D-Bus did not become ready")
}
