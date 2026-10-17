//go:build !linux

package engine

import (
	"context"
	"fmt"
	"net/http"
)

type Options struct {
	Address, State, Runtime, KataConfig, Snapshotter string
	Logger                                           string
	DNS                                              []string
	Retire                                           func(context.Context, string, string) error
	AbortCreate                                      func(context.Context, string, string) error
	Tools                                            map[string]string
}
type Backend struct{}

func Open(Options) (*Backend, error) {
	return nil, fmt.Errorf("the containerd/Dragonball node requires Linux")
}
func (*Backend) Close() error { return nil }
func (*Backend) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "requires Linux", 503)
}
