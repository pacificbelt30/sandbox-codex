package cxd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

// RequestStatus is a durable launch result. Pending and cleanup-pending do not
// imply that it is safe to start another worker with another request ID.
type RequestStatus struct {
	State    string
	WorkerID string
	Name     string
	Attempt  int
}

func requestKey(id string) (string, error) {
	if len(id) == 0 || len(id) > 256 {
		return "", fmt.Errorf("request ID must contain 1..256 bytes")
	}
	sum := sha256.Sum256([]byte(id))
	return hex.EncodeToString(sum[:]), nil
}

// Request looks up a launch attempt without claiming it or transferring its
// lease. A removed completed worker remains visible as a completed request.
func (c *Client) Request(ctx context.Context, requestID string) (RequestStatus, error) {
	key, err := requestKey(requestID)
	if err != nil {
		return RequestStatus{}, err
	}
	r, err := c.node.LaunchStatus(ctx, key)
	if err != nil {
		return RequestStatus{}, classify(err)
	}
	return RequestStatus{State: r.State, WorkerID: r.WorkerID, Name: r.Name, Attempt: r.Attempt}, nil
}

// RemoveRequest retries durable cleanup. A successful return means the worker
// and its cleanup journal are gone, including after a lost previous response.
func (c *Client) RemoveRequest(ctx context.Context, requestID string, force bool) error {
	status, err := c.Request(ctx, requestID)
	if err != nil {
		return err
	}
	if status.WorkerID == "" {
		return fmt.Errorf("%w: request has no worker yet", ErrInProgress)
	}
	err = c.Remove(ctx, status.WorkerID, force)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}
