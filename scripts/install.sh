#!/usr/bin/env bash
# Build and install a matching Oaiso node from this source checkout.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
bundle=""
operator=""
cache=""
apply=false
offline=false

usage() {
  cat <<'EOF'
Usage: scripts/install.sh [--apply] [--operator USER] [--bundle PATH] [--cache PATH] [--offline]

Build a matching node bundle, verify it and show the installation plan.
--apply runs that plan as root after it succeeds. The operator receives
root-equivalent access to the dedicated node; specify one explicitly when
running this script as root. --bundle skips the build and uses a trusted,
previously built bundle.
EOF
}

fail() {
  printf 'install: %s\n' "$1" >&2
  exit 1
}

while (($#)); do
  case "$1" in
    --apply) apply=true ;;
    --offline) offline=true ;;
    --operator|--bundle|--cache)
      (($# >= 2)) || fail "missing value for $1"
      case "$1" in
        --operator) operator="$2" ;;
        --bundle) bundle="$2" ;;
        --cache) cache="$2" ;;
      esac
      shift ;;
    -h|--help) usage; exit 0 ;;
    *) fail "unknown option: $1 (use --help)" ;;
  esac
  shift
done

command -v python3 >/dev/null 2>&1 || fail 'Python 3.11 or newer is required'
python3 -c 'import sys; sys.exit(sys.version_info < (3, 11))' || fail 'Python 3.11 or newer is required'

if [[ -z "$operator" ]]; then
  [[ "$(id -u)" != 0 ]] || fail 'pass --operator USER when running as root'
  operator="$(id -un)"
fi

if [[ -z "$bundle" ]]; then
  command -v go >/dev/null 2>&1 || fail 'Go 1.24.7 or newer is required to build the node bundle'
  command -v make >/dev/null 2>&1 || fail 'Make is required to build the node bundle'
  (cd "$repo_root" && make node-bundle)
  bundle="$repo_root/dist/node-bundle"
fi

[[ -d "$bundle" ]] || fail "bundle directory not found: $bundle"
bundle="$(cd "$bundle" && pwd)"
cli="$bundle/codex-dock"
[[ -x "$cli" && -f "$bundle/bundle.json" ]] || fail "missing CLI or bundle.json in $bundle"

setup_args=(setup --bundle "$bundle" --operator "$operator")
if [[ -n "$cache" ]]; then
  setup_args+=(--cache "$cache")
fi
if "$offline"; then
  setup_args+=(--offline)
fi

"$cli" "${setup_args[@]}"

if "$apply"; then
  if [[ "$(id -u)" == 0 ]]; then
    "$cli" "${setup_args[@]}" --apply
  else
    command -v sudo >/dev/null 2>&1 || fail 'sudo is required for --apply'
    sudo -- "$cli" "${setup_args[@]}" --apply
  fi
  printf 'Node installed. Log in again as %s, then run oaiso init.\n' "$operator"
else
  printf 'Plan only. Re-run scripts/install.sh --apply after reviewing it.\n'
fi
