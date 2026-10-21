//go:build linux

package engine

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	cd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/pkg/cio"
	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
)

type stopContainer struct {
	cd.Container
	task cd.Task
}

func (c stopContainer) Task(context.Context, cio.Attach) (cd.Task, error) { return c.task, nil }

type stopExitTask struct {
	cd.Task
	wait       chan cd.ExitStatus
	exit       *cd.ExitStatus
	terminated bool
	deleted    bool
}

func (t *stopExitTask) Wait(context.Context) (<-chan cd.ExitStatus, error) { return t.wait, nil }
func (t *stopExitTask) Status(context.Context) (cd.Status, error) {
	if t.terminated {
		return cd.Status{}, errdefs.ErrNotFound
	}
	return cd.Status{Status: cd.Running}, nil
}
func (t *stopExitTask) Kill(context.Context, syscall.Signal, ...cd.KillOpts) error {
	t.terminated = true
	if t.exit != nil {
		t.wait <- *t.exit
	}
	close(t.wait)
	return nil
}
func (t *stopExitTask) Delete(context.Context, ...cd.ProcessDeleteOpts) (*cd.ExitStatus, error) {
	t.deleted = true
	return t.exit, nil
}

func TestStopPreservesWaitResultWhenRuntimeForgetsExitedTask(t *testing.T) {
	at := time.Date(2026, 10, 21, 0, 0, 0, 123, time.UTC)
	task := &stopExitTask{wait: make(chan cd.ExitStatus, 1), exit: cd.NewExitStatus(37, at, nil)}
	b := &Backend{opts: Options{State: t.TempDir()}}
	r := &record{ID: strings.Repeat("a", 64), Host: container.HostConfig{Runtime: "runc"}}
	if err := b.stop(context.Background(), r, stopContainer{task: task}, 1); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(b.path(r.ID), "record.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved record
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.ExitCode != 37 || saved.FinishedAt != at.Format(time.RFC3339Nano) {
		t.Fatalf("exit evidence was lost: code=%d time=%s", saved.ExitCode, saved.FinishedAt)
	}
	if task.deleted {
		t.Fatal("deleted a task whose runtime reported it already absent")
	}
}

func TestStopDoesNotClaimSuccessWithoutAnExitResult(t *testing.T) {
	for _, closed := range []bool{false, true} {
		t.Run(map[bool]string{false: "wait-error", true: "closed-channel"}[closed], func(t *testing.T) {
			cause := errors.New("runtime wait failed; process may still be running")
			task := &stopExitTask{wait: make(chan cd.ExitStatus, 1)}
			if !closed {
				task.exit = cd.NewExitStatus(255, time.Time{}, cause)
			}
			b := &Backend{opts: Options{State: t.TempDir()}}
			r := &record{ID: strings.Repeat("b", 64), Host: container.HostConfig{Runtime: "runc"}}
			err := b.stop(context.Background(), r, stopContainer{task: task}, 1)
			if err == nil || !closed && !errors.Is(err, cause) {
				t.Fatalf("missing or failed exit evidence accepted: %v", err)
			}
			if r.FinishedAt != "" || task.deleted {
				t.Fatal("wait failure completed task cleanup")
			}
		})
	}
}
