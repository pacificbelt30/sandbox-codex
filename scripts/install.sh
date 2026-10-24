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
running this script as root. --bundle PATH uses a directory of previously
built CLI/helpers and bundle.json instead of building them here.
Host Python is managed by uv; install uv before running this script.
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

if [[ -z "$operator" ]]; then
  [[ "$(id -u)" != 0 ]] || fail 'pass --operator USER when running as root'
  operator="$(id -un)"
fi

uv=$(command -v uv) || fail 'uv is required; see doc/installation.md'
python_args=(run --no-project --no-config --isolated --managed-python --python '3.12')
if "$offline"; then
  python_args+=(--offline --no-python-downloads)
fi
python=$("$uv" "${python_args[@]}" python -I -c 'import os,sys; print(os.path.realpath(sys._base_executable))') || fail 'cannot prepare uv-managed Python 3.12; for offline setup, prepare it with uv python install 3.12 first'
[[ "$python" = /* && -x "$python" ]] || fail 'uv did not return an absolute Python executable'

if [[ -z "$bundle" ]]; then
  command -v go >/dev/null 2>&1 || fail 'Go 1.26.0 or newer is required to build the node bundle'
  command -v make >/dev/null 2>&1 || fail 'Make is required to build the node bundle'
  (cd "$repo_root" && make node-bundle UV="$uv")
  bundle="$repo_root/dist/node-bundle"
fi

[[ -d "$bundle" ]] || fail "bundle directory not found: $bundle"
bundle="$(cd "$bundle" && pwd)"
cli="$bundle/oaiso"
[[ -x "$cli" && -f "$bundle/bundle.json" ]] || fail "missing CLI or bundle.json in $bundle"

setup_args=(setup --bundle "$bundle" --operator "$operator" --python "$python")
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
