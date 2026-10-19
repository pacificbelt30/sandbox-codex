package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

func TestExecStreamsSeparateOutputAndReturnsExitCode(t *testing.T) {
	var gotCommand []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/v1.51")
		w.Header().Set("Content-Type", "application/json")
		switch path {
		case "/containers/json":
			_ = json.NewEncoder(w).Encode([]container.Summary{{ID: "worker-id", Names: []string{"/worker"}}})
		case "/containers/worker-id/json":
			_ = json.NewEncoder(w).Encode(container.InspectResponse{ContainerJSONBase: &container.ContainerJSONBase{ID: "worker-id", State: &container.State{Running: true}}, Config: &container.Config{Labels: map[string]string{labelManaged: "true", labelIsolation: RuntimeRunc}}})
		case "/containers/worker-id/exec":
			var opts container.ExecOptions
			if err := json.NewDecoder(r.Body).Decode(&opts); err != nil {
				t.Error(err)
			}
			gotCommand = opts.Cmd
			_ = json.NewEncoder(w).Encode(map[string]string{"Id": "exec-id"})
		case "/exec/exec-id/start":
			_, _ = io.Copy(io.Discard, r.Body)
			conn, rw, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			defer conn.Close()
			_, _ = rw.WriteString("HTTP/1.1 101 UPGRADED\r\nContent-Type: application/vnd.docker.raw-stream\r\nConnection: Upgrade\r\n\r\n")
			_ = rw.Flush()
			if len(gotCommand) == 1 && gotCommand[0] == "sleep" {
				select {
				case <-r.Context().Done():
				case <-time.After(time.Second):
				}
				return
			}
			_, _ = io.WriteString(stdcopy.NewStdWriter(conn, stdcopy.Stdout), "normal\n")
			_, _ = io.WriteString(stdcopy.NewStdWriter(conn, stdcopy.Stderr), "error\n")
		case "/exec/exec-id/json":
			_ = json.NewEncoder(w).Encode(container.ExecInspect{ExitCode: 7})
		case "/containers/worker-id/wait":
			_ = json.NewEncoder(w).Encode(container.WaitResponse{StatusCode: 9})
		default:
			t.Errorf("unexpected request: %s", path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	cli, err := client.NewClientWithOpts(client.WithHost("tcp://"+strings.TrimPrefix(server.URL, "http://")), client.WithVersion("1.51"), client.WithHTTPClient(&http.Client{Transport: &http.Transport{}}), client.WithDialContext((&net.Dialer{}).DialContext))
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()
	m := &Manager{cli: cli}
	var out, errOut bytes.Buffer
	code, err := m.Exec(context.Background(), "worker", []string{"sh", "-c", "exit 7"}, nil, &out, &errOut)
	if err != nil {
		t.Fatal(err)
	}
	if code != 7 || out.String() != "normal\n" || errOut.String() != "error\n" || strings.Join(gotCommand, " ") != "sh -c exit 7" {
		t.Fatalf("code=%d stdout=%q stderr=%q argv=%q", code, out.String(), errOut.String(), gotCommand)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = m.Exec(ctx, "worker", []string{"sleep"}, nil, io.Discard, io.Discard)
	if err != context.DeadlineExceeded {
		t.Fatalf("timeout returned %v", err)
	}
	waitCode, err := m.WaitContext(context.Background(), "worker")
	if err != nil || waitCode != 9 {
		t.Fatalf("wait code=%d error=%v", waitCode, err)
	}
}
