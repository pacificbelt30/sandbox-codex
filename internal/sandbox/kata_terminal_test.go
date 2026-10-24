package sandbox

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
)

func TestKataPrimaryTerminal(t *testing.T) {
	if os.Getenv("CODEX_DOCK_KATA_E2E") != "1" {
		t.Skip("set CODEX_DOCK_KATA_E2E=1 on a prepared Linux/KVM host")
	}
	requestDir := os.Getenv("CODEX_DOCK_NODE_RESTART_REQUEST_DIR")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	m, err := NewManager(ManagerConfig{StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Close() }()
	project := t.TempDir()
	id, err := m.RunContext(ctx, RunOptions{Name: "oaiso-terminal-" + randomSuffix(),
		Image: os.Getenv("CODEX_DOCK_KATA_TEST_IMAGE"), ProjectDir: project,
		ContainerUser: fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()), NoInternet: true,
		ExecCmd: `exec python3 -u -c 'import time
from pathlib import Path
p=Path("/workspace/finish")
while not p.exists():
 print("TERMINAL_TICK", flush=True)
 time.sleep(0.1)
print("TERMINAL_FINISHED", flush=True)'`})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := m.Remove(id, true); err != nil {
			t.Error(err)
		}
	}()
	logs := func() string {
		t.Helper()
		stream, err := m.cli.ContainerLogs(ctx, id, container.LogsOptions{ShowStdout: true, ShowStderr: true, Tail: "all"})
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(stream)
		_ = stream.Close()
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	for i := 0; !strings.Contains(logs(), "\nTERMINAL_TICK\r\n"); i++ {
		if i == 100 {
			t.Fatal("terminal did not produce output")
		}
		time.Sleep(100 * time.Millisecond)
	}
	boot := kataExec(t, ctx, m.cli, id, false, []string{"cat", "/proc/sys/kernel/random/boot_id"})
	before := strings.Count(logs(), "\nTERMINAL_TICK\r\n")
	if requestDir != "" {
		if err := os.Remove(filepath.Join(requestDir, "complete")); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(requestDir, "request"), []byte("restart\n"), 0600); err != nil {
			t.Fatal(err)
		}
		for {
			if _, err := os.Stat(filepath.Join(requestDir, "complete")); err == nil {
				break
			}
			select {
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			case <-time.After(100 * time.Millisecond):
			}
		}
		if after := kataExec(t, ctx, m.cli, id, false, []string{"cat", "/proc/sys/kernel/random/boot_id"}); after != boot {
			t.Fatal("terminal VM was recreated")
		}
		time.Sleep(500 * time.Millisecond)
		if strings.Count(logs(), "\nTERMINAL_TICK\r\n") <= before {
			t.Fatal("terminal output did not resume after node restart")
		}
	}
	if err := os.WriteFile(filepath.Join(project, "finish"), []byte("done"), 0600); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-m.Wait(id):
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if !strings.Contains(logs(), "\nTERMINAL_FINISHED\r\n") {
		t.Fatal("final terminal output lost")
	}
	t.Log("verified: primary TTY output and exit; node restart also verified when a restart harness is configured")
}
