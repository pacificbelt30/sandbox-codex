#!/usr/bin/env bash
#
# install.sh — install codex-dock and get it ready to run against Docker.
#
# What it does:
#   1. Checks prerequisites (Go >= 1.24, a reachable Docker daemon).
#   2. Builds the codex-dock binary (from the local checkout if run from inside
#      the repo, otherwise via `go install .../codex-dock@<ref>`).
#   3. Installs the binary to $PREFIX/bin (default: /usr/local/bin).
#   4. Builds the sandbox image (`codex-dock build`) and the auth-proxy image
#      (`codex-dock proxy build`) — the Dockerfiles are embedded in the binary,
#      so this works even without a repo checkout.
#   5. Creates the egress Docker network (`codex-dock network create`).
#
# Usage:
#   ./scripts/install.sh [options]
#   curl -fsSL https://raw.githubusercontent.com/pacificbelt30/codex-dock/main/scripts/install.sh | bash
#
# Options:
#   --prefix DIR       Install prefix (default: /usr/local, binary goes to DIR/bin)
#   --ref REF           go install version/ref to use when not run from a repo checkout (default: latest)
#   --skip-images       Skip building the sandbox/proxy Docker images
#   --skip-network      Skip creating the egress Docker network
#   -h, --help          Show this help
set -euo pipefail

MODULE="github.com/pacificbelt30/codex-dock"
PREFIX="${PREFIX:-/usr/local}"
REF="latest"
SKIP_IMAGES=0
SKIP_NETWORK=0

log()  { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m!!\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31merror:\033[0m %s\n' "$*" >&2; exit 1; }

usage() { sed -n '2,24p' "$0" | sed 's/^# \{0,1\}//'; }

while [ $# -gt 0 ]; do
  case "$1" in
    --prefix) PREFIX="$2"; shift 2 ;;
    --prefix=*) PREFIX="${1#*=}"; shift ;;
    --ref) REF="$2"; shift 2 ;;
    --ref=*) REF="${1#*=}"; shift ;;
    --skip-images) SKIP_IMAGES=1; shift ;;
    --skip-network) SKIP_NETWORK=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown option: $1 (see --help)" ;;
  esac
done

BINDIR="$PREFIX/bin"
BINARY="$BINDIR/codex-dock"

# ── 1. Prerequisites ────────────────────────────────────────────────────────

command -v go >/dev/null 2>&1 || die "Go is required but not found. Install it from https://go.dev/doc/install"

GO_VERSION="$(go env GOVERSION | sed 's/^go//')"
GO_MIN="1.24"
if [ "$(printf '%s\n%s\n' "$GO_MIN" "$GO_VERSION" | sort -V | head -n1)" != "$GO_MIN" ]; then
  die "Go $GO_MIN+ is required, found $GO_VERSION"
fi
log "Go $GO_VERSION found"

command -v docker >/dev/null 2>&1 || die "Docker is required but not found. Install Docker Engine: https://docs.docker.com/engine/install/"

if ! docker version >/dev/null 2>&1; then
  die "Docker is installed but the daemon is not reachable. Start Docker and re-run this script."
fi
log "Docker daemon is reachable"

# ── 2. Build / fetch the binary ─────────────────────────────────────────────

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." 2>/dev/null && pwd || true)"
BUILD_DIR="$(mktemp -d)"
trap 'rm -rf "$BUILD_DIR"' EXIT

if [ -n "$REPO_ROOT" ] && [ -f "$REPO_ROOT/go.mod" ] && grep -q "^module $MODULE\$" "$REPO_ROOT/go.mod" 2>/dev/null; then
  log "Building codex-dock from local checkout ($REPO_ROOT)"
  VERSION="$(git -C "$REPO_ROOT" describe --tags --always --dirty 2>/dev/null || echo dev)"
  ( cd "$REPO_ROOT" && go build -ldflags "-X main.version=$VERSION" -o "$BUILD_DIR/codex-dock" . )
else
  log "Fetching codex-dock@$REF via go install"
  GOBIN="$BUILD_DIR" go install "${MODULE}@${REF}"
fi

[ -x "$BUILD_DIR/codex-dock" ] || die "build failed: $BUILD_DIR/codex-dock not found"

# ── 3. Install the binary ───────────────────────────────────────────────────

log "Installing to $BINARY"
if [ -w "$PREFIX" ] || [ -w "$BINDIR" ] 2>/dev/null; then
  install -d "$BINDIR"
  install -m 0755 "$BUILD_DIR/codex-dock" "$BINARY"
else
  command -v sudo >/dev/null 2>&1 || die "$BINDIR is not writable and sudo is not available; re-run with --prefix pointing at a writable directory"
  sudo install -d "$BINDIR"
  sudo install -m 0755 "$BUILD_DIR/codex-dock" "$BINARY"
fi

case ":$PATH:" in
  *":$BINDIR:"*) ;;
  *) warn "$BINDIR is not on your PATH — add it (e.g. export PATH=\"$BINDIR:\$PATH\")" ;;
esac

# ── 4. Docker images ─────────────────────────────────────────────────────────

if [ "$SKIP_IMAGES" -eq 0 ]; then
  log "Building sandbox image (codex-dock build)"
  "$BINARY" build

  log "Building auth proxy image (codex-dock proxy build)"
  "$BINARY" proxy build
else
  log "Skipping Docker image builds (--skip-images)"
fi

# ── 5. Egress network ────────────────────────────────────────────────────────

if [ "$SKIP_NETWORK" -eq 0 ]; then
  log "Creating egress network (codex-dock network create)"
  "$BINARY" network create
else
  log "Skipping egress network creation (--skip-network)"
fi

log "codex-dock installed: $("$BINARY" --help >/dev/null 2>&1 && echo "$BINARY")"
cat <<EOF

Next steps:
  export OPENAI_API_KEY=sk-...        # for the Codex agent
  export ANTHROPIC_API_KEY=sk-ant-... # for the Claude agent
  codex-dock auth set
  codex-dock proxy run
  codex-dock run --agent codex --approval-mode full-auto
  codex-dock run --agent claude --approval-mode full-auto

EOF
