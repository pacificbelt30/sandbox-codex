package cmd

import (
	"context"
	"debug/elf"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/pacificbelt30/codex-dock/internal/engine"
)

// An installed Linux CLI bundle is static and already matches the node. Build
// its proxy image directly from those bytes, so a project clone need not carry
// the cxd source checkout just to refresh a stale proxy image.
func buildProxyImageFromExecutable(ctx context.Context, tag string) (bool, error) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		return false, nil
	}
	executable, err := os.Executable()
	if err != nil {
		return false, err
	}
	file, err := elf.Open(executable)
	if err != nil {
		return false, nil
	}
	static := file.Machine == elf.EM_X86_64
	for _, program := range file.Progs {
		if program.Type == elf.PT_INTERP {
			static = false
			break
		}
	}
	if err := file.Close(); err != nil {
		return false, err
	}
	if !static {
		return false, nil
	}
	dir, err := os.MkdirTemp("", "cxd-proxy-image-*")
	if err != nil {
		return true, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	src, err := os.Open(executable)
	if err != nil {
		return true, err
	}
	defer func() { _ = src.Close() }()
	copyPath := filepath.Join(dir, "codex-dock")
	dst, err := os.OpenFile(copyPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0755)
	if err != nil {
		return true, err
	}
	if _, err = io.Copy(dst, src); err != nil {
		_ = dst.Close()
		return true, err
	}
	if err = dst.Close(); err != nil {
		return true, err
	}
	dockerfile := filepath.Join(dir, "Dockerfile")
	contents := fmt.Sprintf(`FROM gcr.io/distroless/base-debian12
ARG CODEX_DOCK_BUILD_VERSION=dev
LABEL %s=%q
LABEL %s=$CODEX_DOCK_BUILD_VERSION
COPY codex-dock /codex-dock
ENTRYPOINT ["/codex-dock"]
CMD ["proxy", "serve", "--listen", "0.0.0.0:18080"]
`, proxyImageProtocolLabel, proxyImageProtocolVersion, proxyImageBuildLabel)
	if err := os.WriteFile(dockerfile, []byte(contents), 0644); err != nil {
		return true, err
	}
	return true, engine.Build(ctx, tag, dockerfile, dir, os.Stderr)
}
