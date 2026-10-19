package cxd

import (
	"context"
	"io"
)

const MaxFileBytes = 64 << 20

func (c *Client) ReadFile(ctx context.Context, id, path string, out io.Writer) error {
	return classify(c.manager.ReadFile(ctx, id, path, out))
}
func (c *Client) WriteFile(ctx context.Context, id, path string, in io.Reader) error {
	return classify(c.manager.WriteFile(ctx, id, path, in))
}
func (c *Client) ListFiles(ctx context.Context, id, path string, out io.Writer) error {
	return classify(c.manager.ListFiles(ctx, id, path, out))
}
func (c *Client) ExportDiff(ctx context.Context, id string, out io.Writer) error {
	return classify(c.manager.ExportDiff(ctx, id, out))
}
func (c *Client) Commit(ctx context.Context, id, message string) error {
	return classify(c.manager.Commit(ctx, id, message))
}
