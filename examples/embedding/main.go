// A minimal host application using the public API. Prepare a matching node,
// images and auth proxy first; see doc/go-api.md. No credentials enter the VM.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"time"

	"github.com/pacificbelt30/codex-dock/pkg/cxd"
	"github.com/pacificbelt30/codex-dock/pkg/cxd/identity"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context) (result error) {
	project := flag.String("project", "", "absolute host project directory")
	requestID := flag.String("request-id", "", "unique durable ID for this attempt; this example removes it at exit")
	image := flag.String("image", "codex-dock:latest", "preloaded worker image")
	profile := flag.String("profile", "compact", "registered node resource profile")
	model := flag.String("model", "", "optional per-token allowed model")
	prompt := flag.String("prompt", "", "optional Codex task; uses the configured upstream and may incur charges")
	temporaryMiB := flag.Int64("temporary-mib", 0, "optional VM-lifetime tmpfs ceiling in MiB")
	flag.Parse()
	if !filepath.IsAbs(*project) || *requestID == "" {
		return fmt.Errorf("supply -project with an absolute path and -request-id")
	}
	if *prompt != "" && *model == "" {
		return fmt.Errorf("-prompt requires -model")
	}
	if *temporaryMiB < 0 || *temporaryMiB > 2048 {
		return fmt.Errorf("-temporary-mib must be within 0..2048")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	client, err := cxd.Open(ctx, cxd.Options{
		NodeSocket:        os.Getenv("CODEX_DOCK_KATA_NODE_SOCKET"),
		ProxyAdminURL:     os.Getenv("CXD_EXAMPLE_PROXY_ADMIN_URL"),
		ProxyContainerURL: os.Getenv("CXD_EXAMPLE_PROXY_CONTAINER_URL"),
		ProxyAdminSecret:  os.Getenv("CODEX_DOCK_PROXY_ADMIN_SECRET"),
		StateDir:          os.Getenv("CXD_EXAMPLE_STATE_DIR"),
	})
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, client.Close()) }()
	caps := client.Capabilities()
	for _, feature := range []string{"create-before-start-v1", "exec-options-v1"} {
		if !slices.Contains(caps.Features, feature) {
			return fmt.Errorf("node requires %s", feature)
		}
	}
	profiles, err := client.Profiles(ctx)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(profiles, func(p cxd.Profile) bool { return p.Name == *profile }) {
		return fmt.Errorf("profile %q is not registered on this node", *profile)
	}
	req := cxd.LaunchRequest{
		RequestID: *requestID, ProjectDir: *project, Image: *image, Profile: *profile,
		NoInternet: true, NoGitMetadata: true,
		User: fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()),
		Identity: identity.Identity{
			CaseID: os.Getenv("CXD_EXAMPLE_CASE_ID"), RunID: os.Getenv("CXD_EXAMPLE_RUN_ID"),
			ConnectionVersion: os.Getenv("CXD_EXAMPLE_CONNECTION_VERSION"),
		},
	}
	if !req.Identity.Empty() {
		req.Identity.Provider = "openai"
		if err := req.Identity.Validate(); err != nil {
			return err
		}
	}
	if *model != "" {
		proxyCaps, err := client.ProxyCapabilities(ctx)
		if err != nil {
			return err
		}
		if !proxyCaps.TokenModelPolicy {
			return fmt.Errorf("proxy does not support per-token model authorization")
		}
		req.ModelPolicy = &identity.ModelPolicy{AllowedModels: []string{*model}}
	}
	if *temporaryMiB != 0 {
		req.TemporaryStorage = []cxd.TemporaryStorage{{Target: "/run/build-cache", MaxBytes: *temporaryMiB << 20, Executable: true}}
	}
	worker, err := client.Create(ctx, req)
	// An error with an ID can leave cleanup work after an uncertain response.
	// A conflicting request without an ID is never removed by this example.
	if err == nil || worker.ID != "" {
		defer func() {
			cleanup, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			if err := client.RemoveRequest(cleanup, req.RequestID, true); err != nil {
				result = errors.Join(result, fmt.Errorf("cleanup request %q: %w", req.RequestID, err))
			} else {
				fmt.Println("REMOVE_OK")
			}
		}()
	}
	if err != nil {
		return err
	}
	fmt.Println("CREATE_OK")
	if err := client.Start(ctx, worker.ID); err != nil {
		return err
	}
	fmt.Println("START_OK")
	connections, err := client.Connections(ctx, worker.ID)
	if err != nil {
		return err
	}
	for _, c := range connections {
		fmt.Printf("connection provider=%s transports=%v model_policy=%t\n", c.Provider, c.Transports, c.ModelPolicyActive)
	}
	// Cancel one exec, then prove the worker can still run another command.
	call, stopCall := context.WithTimeout(ctx, 30*time.Second)
	_, err = client.Exec(call, worker.ID, cxd.ExecRequest{
		Argv:   []string{"python3", "-u", "-c", "import time; print('streaming before cancellation', flush=True); time.sleep(120)"},
		Stdout: cancelOnOutput{Writer: os.Stdout, cancel: stopCall}, Stderr: os.Stderr,
	})
	stopCall()
	if !errors.Is(err, context.Canceled) {
		return fmt.Errorf("expected exec cancellation after output, got %v", err)
	}
	if err := execute(ctx, client, worker.ID, cxd.ExecRequest{Argv: []string{"printf", "EXEC_CANCEL_OK\n"}, Stdout: os.Stdout, Stderr: os.Stderr}); err != nil {
		return err
	}
	if *prompt != "" {
		command, err := client.CodexHTTPCommand(ctx, worker.ID)
		if err != nil {
			return err
		}
		command.Argv = append(command.Argv, "exec", "--skip-git-repo-check", "-m", *model, *prompt)
		// Env contains a short-lived placeholder: pass it through, never log it.
		if err := execute(ctx, client, worker.ID, cxd.ExecRequest{Argv: command.Argv, Env: command.Env, Stdout: os.Stdout, Stderr: os.Stderr}); err != nil {
			return err
		}
		fmt.Println("CODEX_HTTP_OK")
	}
	return client.Stop(ctx, worker.ID, 5*time.Second)
}

type cancelOnOutput struct {
	io.Writer
	cancel context.CancelFunc
}

func (w cancelOnOutput) Write(data []byte) (int, error) {
	n, err := w.Writer.Write(data)
	if n > 0 {
		w.cancel()
	}
	return n, err
}

func execute(ctx context.Context, client *cxd.Client, worker string, request cxd.ExecRequest) error {
	code, err := client.Exec(ctx, worker, request)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("guest command exited with code %d", code)
	}
	return nil
}
