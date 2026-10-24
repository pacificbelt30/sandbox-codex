package sandbox

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/docker/docker/client"
)

func TestWaitKeepsFinalForegroundOutputAlive(t *testing.T) {
	served := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"StatusCode":0}`))
		close(served)
	}))
	defer server.Close()
	c, err := client.NewClientWithOpts(client.WithHost(server.URL), client.WithVersion("1.51"),
		client.WithHTTPClient(&http.Client{Transport: &http.Transport{}}))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	outputDone := make(chan struct{})
	m := &Manager{cli: c, attachments: map[string]chan struct{}{"fixture": outputDone}}
	done := m.Wait("fixture")
	<-served
	select {
	case err := <-done:
		t.Fatalf("returned before output completed: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	close(outputDone)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("did not finish after output drained")
	}
}
