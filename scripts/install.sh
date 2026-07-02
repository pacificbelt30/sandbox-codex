#!/usr/bin/env bash
#
# install.sh — codex-dock をインストールし、Docker ですぐ使える状態にする。
#
# 実行内容:
#   1. 前提条件を確認する（Go 1.24 以上、Docker デーモンへの疎通）。
#   2. codex-dock バイナリをビルドする（リポジトリ内から実行した場合はローカル
#      ビルド、それ以外は `go install .../codex-dock@<ref>` で取得）。
#   3. バイナリを $PREFIX/bin（既定: /usr/local/bin）に配置する。
#   4. サンドボックスイメージ（`codex-dock build`）と Auth Proxy イメージ
#      （`codex-dock proxy build`）をビルドする — Dockerfile はバイナリに
#      埋め込まれているため、リポジトリを clone していなくても実行できる。
#   5. egress ネットワークを作成する（`codex-dock network create`）。
#
# 使い方:
#   ./scripts/install.sh [options]
#   curl -fsSL https://raw.githubusercontent.com/pacificbelt30/codex-dock/main/scripts/install.sh | bash
#
# オプション:
#   --prefix DIR       インストール先（既定: /usr/local、バイナリは DIR/bin に配置）
#   --ref REF           リポジトリ未 clone 時に `go install` で使うバージョン/ref（既定: latest）
#   --skip-images       Docker イメージのビルドをスキップする
#   --skip-network      egress ネットワークの作成をスキップする
#   -h, --help          このヘルプを表示する
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
    *) die "不明なオプションです: $1 (--help を参照)" ;;
  esac
done

BINDIR="$PREFIX/bin"
BINARY="$BINDIR/codex-dock"

# ── 1. 前提条件の確認 ──────────────────────────────────────────────────────

command -v go >/dev/null 2>&1 || die "Go が見つかりません。https://go.dev/doc/install からインストールしてください"

GO_VERSION="$(go env GOVERSION | sed 's/^go//')"
GO_MIN="1.24"
if [ "$(printf '%s\n%s\n' "$GO_MIN" "$GO_VERSION" | sort -V | head -n1)" != "$GO_MIN" ]; then
  die "Go $GO_MIN 以上が必要です（検出されたバージョン: $GO_VERSION）"
fi
log "Go $GO_VERSION を検出しました"

command -v docker >/dev/null 2>&1 || die "Docker が見つかりません。Docker Engine をインストールしてください: https://docs.docker.com/engine/install/"

if ! docker version >/dev/null 2>&1; then
  die "Docker はインストールされていますが、デーモンに接続できません。Docker を起動してから再実行してください。"
fi
log "Docker デーモンに接続できました"

# ── 2. バイナリのビルド / 取得 ─────────────────────────────────────────────

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." 2>/dev/null && pwd || true)"
BUILD_DIR="$(mktemp -d)"
trap 'rm -rf "$BUILD_DIR"' EXIT

if [ -n "$REPO_ROOT" ] && [ -f "$REPO_ROOT/go.mod" ] && grep -q "^module $MODULE\$" "$REPO_ROOT/go.mod" 2>/dev/null; then
  log "ローカルの checkout からビルドします ($REPO_ROOT)"
  VERSION="$(git -C "$REPO_ROOT" describe --tags --always --dirty 2>/dev/null || echo dev)"
  ( cd "$REPO_ROOT" && go build -ldflags "-X main.version=$VERSION" -o "$BUILD_DIR/codex-dock" . )
else
  log "go install で codex-dock@$REF を取得します"
  GOBIN="$BUILD_DIR" go install "${MODULE}@${REF}"
fi

[ -x "$BUILD_DIR/codex-dock" ] || die "ビルドに失敗しました: $BUILD_DIR/codex-dock が見つかりません"

# ── 3. バイナリの配置 ───────────────────────────────────────────────────────

log "$BINARY に配置します"
if [ -w "$PREFIX" ] || [ -w "$BINDIR" ] 2>/dev/null; then
  install -d "$BINDIR"
  install -m 0755 "$BUILD_DIR/codex-dock" "$BINARY"
else
  command -v sudo >/dev/null 2>&1 || die "$BINDIR に書き込み権限がなく、sudo も利用できません。書き込み可能なディレクトリを --prefix で指定して再実行してください"
  sudo install -d "$BINDIR"
  sudo install -m 0755 "$BUILD_DIR/codex-dock" "$BINARY"
fi

case ":$PATH:" in
  *":$BINDIR:"*) ;;
  *) warn "$BINDIR が PATH に含まれていません（例: export PATH=\"$BINDIR:\$PATH\"）" ;;
esac

# ── 4. Docker イメージのビルド ───────────────────────────────────────────────

if [ "$SKIP_IMAGES" -eq 0 ]; then
  log "サンドボックスイメージをビルドします (codex-dock build)"
  "$BINARY" build

  log "Auth Proxy イメージをビルドします (codex-dock proxy build)"
  "$BINARY" proxy build
else
  log "Docker イメージのビルドをスキップします (--skip-images)"
fi

# ── 5. egress ネットワークの作成 ─────────────────────────────────────────────

if [ "$SKIP_NETWORK" -eq 0 ]; then
  log "egress ネットワークを作成します (codex-dock network create)"
  "$BINARY" network create
else
  log "egress ネットワークの作成をスキップします (--skip-network)"
fi

log "codex-dock のインストールが完了しました: $("$BINARY" --help >/dev/null 2>&1 && echo "$BINARY")"
cat <<EOF

次のステップ:
  export OPENAI_API_KEY=sk-...        # Codex エージェント用
  export ANTHROPIC_API_KEY=sk-ant-... # Claude エージェント用
  codex-dock auth set
  codex-dock proxy run
  codex-dock run --agent codex --approval-mode full-auto
  codex-dock run --agent claude --approval-mode full-auto

EOF
