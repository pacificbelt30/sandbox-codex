//go:build linux

package engine

import (
	"context"
	"encoding/json"
	"fmt"
	cd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/containerd/errdefs"
	"github.com/opencontainers/runtime-spec/specs-go"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

const jobOutputLimit int64 = 16 << 20

func (b *Backend) jobDir(e *execution) string {
	return filepath.Join(b.path(e.Container), "jobs", e.ID)
}
func (b *Backend) saveJob(e *execution) error {
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	dir := b.jobDir(e)
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".job-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err = f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(dir, "job.json"))
}

// Called with b.mu held. Logging belongs to the shim and survives node restarts.
func (b *Backend) startJob(ctx context.Context, e *execution, task cd.Task, base *specs.Process) error {
	if e.Config.AttachStdin || e.Config.Tty {
		return fmt.Errorf("background jobs cannot attach stdin or PTY; use a streaming session")
	}
	if err := b.saveJob(e); err != nil {
		return err
	}
	creator, err := b.logIOAt(b.jobDir(e), e.ID, false, false, jobOutputLimit)
	if err != nil {
		return err
	}
	spec := *base
	spec.Terminal = false
	spec.Args = e.Config.Cmd
	spec.Env = append(append([]string{}, base.Env...), e.Config.Env...)
	if e.Config.WorkingDir != "" {
		spec.Cwd = e.Config.WorkingDir
	}
	p, err := task.Exec(ctx, e.ID, &spec, creator)
	if err != nil {
		return err
	}
	waitCtx := namespaces.WithNamespace(context.Background(), Namespace)
	wait, err := p.Wait(waitCtx)
	if err != nil {
		_, _ = p.Delete(ctx, cd.WithProcessKill)
		return err
	}
	if err = p.Start(ctx); err != nil {
		_, _ = p.Delete(ctx, cd.WithProcessKill)
		return err
	}
	if closer, ok := p.IO().(interface{ CloseInput() error }); ok {
		if err = closer.CloseInput(); err != nil {
			_, _ = p.Delete(ctx, cd.WithProcessKill)
			return err
		}
	}
	if err = p.CloseIO(ctx, cd.WithStdinCloser); err != nil && !errdefs.IsNotFound(err) {
		_, _ = p.Delete(ctx, cd.WithProcessKill)
		return err
	}
	go b.watchJob(e, p, wait)
	return nil
}
func (b *Backend) watchJob(e *execution, p cd.Process, wait <-chan cd.ExitStatus) {
	status := <-wait
	code, at, err := status.Result()
	if err != nil {
		code = 125
		at = time.Now().UTC()
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || b.execs[e.ID] != e {
		return
	}
	e.Running = false
	e.ExitCode = int(code)
	e.FinishedAt = at
	ctx, cancel := context.WithTimeout(namespaces.WithNamespace(context.Background(), Namespace), 10*time.Second)
	defer cancel()
	if rec, _, err := b.load(ctx, e.Container); err == nil {
		rec.LastActivity = time.Now().UTC()
		_ = b.save(rec)
	}
	_ = b.saveJob(e)
	_, _ = p.Delete(ctx, cd.WithProcessKill)
	// Completed jobs persist until their worker is removed. Limits prevent an
	// unbounded number of retained sessions; explicit cleanup can remove a worker.
}
func (b *Backend) recoverJobs(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	entries, err := os.ReadDir(filepath.Join(b.opts.State, "tasks"))
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() || !hexID.MatchString(entry.Name()) {
			continue
		}
		jobs, err := os.ReadDir(filepath.Join(b.path(entry.Name()), "jobs"))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		for _, job := range jobs {
			if !job.IsDir() || !hexID.MatchString(job.Name()) {
				continue
			}
			data, err := os.ReadFile(filepath.Join(b.path(entry.Name()), "jobs", job.Name(), "job.json"))
			if err != nil {
				return err
			}
			var e execution
			if err = json.Unmarshal(data, &e); err != nil {
				return err
			}
			if e.ID != job.Name() || e.Container != entry.Name() || !e.Detached {
				return fmt.Errorf("invalid retained job identity")
			}
			if len(b.execs) >= 4096 {
				return fmt.Errorf("too many retained jobs")
			}
			b.execs[e.ID] = &e
			if !e.Running {
				continue
			}
			_, c, err := b.load(ctx, e.Container)
			if err != nil {
				return err
			}
			task, err := c.Task(ctx, nil)
			if errdefs.IsNotFound(err) {
				e.Running = false
				e.ExitCode = 125
				_ = b.saveJob(&e)
				continue
			}
			if err != nil {
				return err
			}
			p, err := task.LoadProcess(ctx, e.ID, nil)
			if errdefs.IsNotFound(err) {
				e.Running = false
				e.ExitCode = 125
				_ = b.saveJob(&e)
				continue
			}
			if err != nil {
				return err
			}
			wait, err := p.Wait(namespaces.WithNamespace(context.Background(), Namespace))
			if err != nil {
				return err
			}
			go b.watchJob(&e, p, wait)
		}
	}
	return nil
}
func (b *Backend) jobAction(w http.ResponseWriter, r *http.Request, ctx context.Context, eid, action string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	e := b.execs[eid]
	if e == nil {
		http.NotFound(w, r)
		return
	}
	if action == "logs" {
		if !e.Detached {
			failure(w, fmt.Errorf("logs require a background job"))
			return
		}
		offset, _ := strconv.ParseInt(r.URL.Query().Get("offset"), 10, 64)
		if offset < 0 || offset > jobOutputLimit {
			failure(w, fmt.Errorf("invalid log offset"))
			return
		}
		file, err := os.Open(filepath.Join(b.jobDir(e), "console.log"))
		if os.IsNotExist(err) {
			reply(w, map[string]any{"output": "", "next": offset, "running": e.Running})
			return
		}
		if err != nil {
			failure(w, err)
			return
		}
		defer func() { _ = file.Close() }()
		if _, err = file.Seek(offset, io.SeekStart); err != nil {
			failure(w, err)
			return
		}
		data, err := io.ReadAll(io.LimitReader(file, 1<<20))
		if err != nil {
			failure(w, err)
			return
		}
		info, err := file.Stat()
		if err != nil {
			failure(w, err)
			return
		}
		reply(w, map[string]any{"output": string(data), "next": offset + int64(len(data)), "running": e.Running, "truncated": info.Size() >= jobOutputLimit})
		return
	}
	_, c, err := b.load(ctx, e.Container)
	if err != nil {
		failure(w, err)
		return
	}
	task, err := c.Task(ctx, nil)
	if err != nil {
		failure(w, err)
		return
	}
	p, err := task.LoadProcess(ctx, e.ID, nil)
	if err != nil {
		failure(w, err)
		return
	}
	switch action {
	case "kill":
		if r.Method != "POST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		err = p.Kill(ctx, 9)
	case "resize":
		width, e1 := strconv.ParseUint(r.URL.Query().Get("w"), 10, 32)
		height, e2 := strconv.ParseUint(r.URL.Query().Get("h"), 10, 32)
		if e1 != nil || e2 != nil || width == 0 || height == 0 || width > 10000 || height > 10000 {
			failure(w, fmt.Errorf("invalid PTY dimensions"))
			return
		}
		err = p.Resize(ctx, uint32(width), uint32(height))
	}
	if err != nil {
		failure(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
