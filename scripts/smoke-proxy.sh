#!/usr/bin/env bash
#
# smoke-proxy.sh — live smoke test of the codex-dock auth proxy / router.
#
# Exercises the running binary (no Docker required) to verify:
#   1. data-plane /health is reachable
#   2. /admin/* is served on the dedicated admin listener
#   3. /admin/* is NOT served on the worker-facing data-plane port (split)
#   4. the HTTP forward proxy forwards absolute-form requests
#   5. the CONNECT forward proxy tunnels connections
#   6. the forward-proxy domain allowlist blocks disallowed hosts (403)
#
# Requires: go, python3, curl. The full container behaviour (worker<->worker
# isolation, Internal-network egress blocking) still needs a Docker daemon and
# is covered by the manual end-to-end steps in doc/network.md.
set -u

# Detach from any controlling-terminal stdin so backgrounded servers don't get
# stopped by SIGTTIN/SIGTTOU when launched from a non-interactive harness.
exec </dev/null

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
TMP="$(mktemp -d)"
BIN="$TMP/codex-dock"
DP=18080; ADM=18081; ORIGIN=18090; DENY=18180
PASS=0; FAIL=0
ok(){ echo "PASS: $1"; PASS=$((PASS+1)); }
no(){ echo "FAIL: $1"; FAIL=$((FAIL+1)); }

# This environment may export NO_PROXY/HTTP(S)_PROXY for an outer agent proxy.
# curl honours NO_PROXY even with -x, which would make it bypass our proxy for
# 127.0.0.1 targets. Neutralise those so curl actually traverses our proxy.
CURL="env -u NO_PROXY -u no_proxy -u HTTP_PROXY -u HTTPS_PROXY -u http_proxy -u https_proxy curl -s --noproxy "

PIDS=()
cleanup(){ for p in "${PIDS[@]:-}"; do kill "$p" 2>/dev/null; done; rm -rf "$TMP"; }
trap cleanup EXIT

echo "=== build ==="
go build -o "$BIN" . || { echo "build failed"; exit 1; }

python3 -m http.server "$ORIGIN" --bind 127.0.0.1 >/dev/null 2>&1 & PIDS+=($!)
"$BIN" proxy serve --listen 127.0.0.1:$DP --admin-listen 127.0.0.1:$ADM >/dev/null 2>&1 & PIDS+=($!)
"$BIN" proxy serve --listen 127.0.0.1:$DENY --forward-allow-domain example.com >/dev/null 2>&1 & PIDS+=($!)

# Wait for readiness (curl retries on connection-refused; no foreground sleep).
# The origin server must be waited on too, or the forward-proxy tests race it.
$CURL '' --retry-connrefused --retry 30 --retry-delay 0 "http://127.0.0.1:$ORIGIN/go.mod" >/dev/null 2>&1
$CURL '' --retry-connrefused --retry 30 --retry-delay 0 "http://127.0.0.1:$DP/health" >/dev/null 2>&1
$CURL '' --retry-connrefused --retry 30 --retry-delay 0 "http://127.0.0.1:$ADM/admin/mode" >/dev/null 2>&1
$CURL '' --retry-connrefused --retry 30 --retry-delay 0 "http://127.0.0.1:$DENY/health" >/dev/null 2>&1

c=$($CURL '' -o /dev/null -w '%{http_code}' "http://127.0.0.1:$DP/health")
[ "$c" = 200 ] && ok "data-plane /health = 200" || no "data-plane /health = $c"

if $CURL '' "http://127.0.0.1:$ADM/admin/mode" | grep -q anthropic_available; then
  ok "admin listener serves /admin/mode"; else no "admin listener missing /admin/mode"; fi

c=$($CURL '' -o /dev/null -w '%{http_code}' "http://127.0.0.1:$DP/admin/mode")
[ "$c" = 404 ] && ok "data-plane /admin/mode = 404 (admin isolated from workers' port)" \
                || no "data-plane /admin/mode = $c (should be 404)"

if $CURL '' -x "http://127.0.0.1:$DP" "http://127.0.0.1:$ORIGIN/go.mod" | grep -q "pacificbelt30/codex-dock"; then
  ok "HTTP forward proxy returned origin content"; else no "HTTP forward proxy failed"; fi

c=$($CURL '' -o /dev/null -w '%{http_code}' -x "http://127.0.0.1:$DP" --proxytunnel "http://127.0.0.1:$ORIGIN/go.mod")
[ "$c" = 200 ] && ok "CONNECT tunnel = 200" || no "CONNECT tunnel = $c"

c=$($CURL '' -o /dev/null -w '%{http_code}' -x "http://127.0.0.1:$DENY" "http://127.0.0.1:$ORIGIN/go.mod")
[ "$c" = 403 ] && ok "allowlist blocks disallowed host (403)" || no "allowlist did not block (got $c)"

echo
echo "=== RESULT: PASS=$PASS FAIL=$FAIL ==="
[ "$FAIL" = 0 ]
