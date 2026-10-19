package cxd

import (
	"errors"
	"fmt"

	"github.com/containerd/errdefs"
	"github.com/pacificbelt30/codex-dock/internal/kataguard"
)

// Error categories are stable for errors.Is checks. The wrapped cause retains
// node and OS details for logging and diagnosis.
var (
	ErrNotFound        = errors.New("cxd: worker not found")
	ErrConflict        = errors.New("cxd: request ID conflicts with existing launch")
	ErrInProgress      = errors.New("cxd: launch is in progress")
	ErrCleanupPending  = errors.New("cxd: cleanup is pending")
	ErrRequestGone     = errors.New("cxd: completed request worker was removed")
	ErrCapacity        = errors.New("cxd: node capacity exhausted")
	ErrVersionMismatch = errors.New("cxd: client and node versions differ")
	ErrOutcomeUnknown  = errors.New("cxd: operation outcome needs reconciliation")
)

func classify(err error) error {
	if err == nil {
		return nil
	}
	var version *kataguard.VersionMismatchError
	if errors.As(err, &version) {
		return fmt.Errorf("%w: %w", ErrVersionMismatch, err)
	}
	var node *kataguard.NodeError
	if errors.As(err, &node) {
		switch node.Code {
		case "request_conflict":
			return fmt.Errorf("%w: %w", ErrConflict, err)
		case "capacity_exhausted":
			return fmt.Errorf("%w: %w", ErrCapacity, err)
		}
	}
	if errdefs.IsNotFound(err) {
		return fmt.Errorf("%w: %w", ErrNotFound, err)
	}
	return err
}
