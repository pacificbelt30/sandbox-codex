#!/usr/bin/env bash
#
# smoke-proxy.sh — live smoke test of the codex-dock proxies (no Docker needed).
#
# Two roles run as separate processes here (in production: separate containers):
#   - auth   (codex-auth-proxy):  reverse routes + token + admin; NO forwarding.
#   - egress (codex-http-proxy):  forward proxy only; no credentials; optional
#                                 private/LAN block + domain allowlist.
#
# Verifies:
#   1. auth /health reachable
#   2. auth /admin/mode on the dedicated admin listener
#   3. auth /admin/* NOT served on the data-plane port (split)
#   4. auth REFUSES to forward general traffic (405) — credential holder is not a proxy
#   5. egress forwards HTTP and CONNECT to an origin
#   6. egress with --block-private refuses a private/LAN destination (403 / blocked)
#
# (The forward-proxy domain allowlist is covered by the Go unit tests.)
#
# Requires: go, python3, curl. Container-level isolation (worker<->worker,
# Internal-network egress) still needs a Docker daemon (see doc/network.md).
set -u

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
TMP="$(mktemp -d)"
BIN="$TMP/codex-dock"
AUTH=18080; ADMIN=18081; EGRESS=18082; EGRESS_BLOCK=18083; ORIGIN=18090
PASS=0; FAIL=0; REPORT=""
ok(){ REPORT+="PASS: $1"$'\n'; PASS=$((PASS+1)); }
no(){ REPORT+="FAIL: $1"$'\n'; FAIL=$((FAIL+1)); }

# Neutralise any outer-agent NO_PROXY/HTTP(S)_PROXY so curl actually traverses our proxy.
CURL="env -u NO_PROXY -u no_proxy -u HTTP_PROXY -u HTTPS_PROXY -u http_proxy -u https_proxy curl -s --noproxy "

echo "=== build ==="
go build -o "$BIN" . || { rm -rf "$TMP"; echo "build failed"; exit 1; }

# Each server reads stdin from /dev/null so a non-interactive shell doesn't stop
# it with SIGTTIN/SIGTTOU.
python3 -m http.server "$ORIGIN" --bind 127.0.0.1 >/dev/null 2>&1 </dev/null & P_ORIGIN=$!
"$BIN" proxy serve --role auth   --listen 127.0.0.1:$AUTH --admin-listen 127.0.0.1:$ADMIN >/dev/null 2>&1 </dev/null & P_AUTH=$!
"$BIN" proxy serve --role egress --listen 127.0.0.1:$EGRESS >/dev/null 2>&1 </dev/null & P_EGRESS=$!
"$BIN" proxy serve --role egress --listen 127.0.0.1:$EGRESS_BLOCK --block-private >/dev/null 2>&1 </dev/null & P_BLOCK=$!

# Wait for readiness (curl retries on connection-refused; no foreground sleep).
for p in $ORIGIN $AUTH $EGRESS $EGRESS_BLOCK; do
  $CURL '' --retry-connrefused --retry 40 --retry-delay 0 "http://127.0.0.1:$p/health" >/dev/null 2>&1 || true
done

c=$($CURL '' -o /dev/null -w '%{http_code}' "http://127.0.0.1:$AUTH/health")
[ "$c" = 200 ] && ok "auth /health = 200" || no "auth /health = $c"

if $CURL '' "http://127.0.0.1:$ADMIN/admin/mode" | grep -q anthropic_available; then
  ok "auth admin listener serves /admin/mode"; else no "auth admin listener missing /admin/mode"; fi

c=$($CURL '' -o /dev/null -w '%{http_code}' "http://127.0.0.1:$AUTH/admin/mode")
[ "$c" = 404 ] && ok "auth data-plane /admin/mode = 404 (admin isolated)" || no "auth data-plane /admin/mode = $c (want 404)"

c=$($CURL '' -o /dev/null -w '%{http_code}' -x "http://127.0.0.1:$AUTH" "http://127.0.0.1:$ORIGIN/go.mod")
[ "$c" = 405 ] && ok "auth refuses to forward general traffic (405)" || no "auth forwarded general traffic (got $c, want 405)"

if $CURL '' -x "http://127.0.0.1:$EGRESS" "http://127.0.0.1:$ORIGIN/go.mod" | grep -q "pacificbelt30/codex-dock"; then
  ok "egress HTTP forward returned origin content"; else no "egress HTTP forward failed"; fi
c=$($CURL '' -o /dev/null -w '%{http_code}' -x "http://127.0.0.1:$EGRESS" --proxytunnel "http://127.0.0.1:$ORIGIN/go.mod")
[ "$c" = 200 ] && ok "egress CONNECT tunnel = 200" || no "egress CONNECT tunnel = $c"

c=$($CURL '' -o /dev/null -w '%{http_code}' -x "http://127.0.0.1:$EGRESS_BLOCK" "http://127.0.0.1:$ORIGIN/go.mod")
[ "$c" = 403 ] && ok "egress --block-private blocks LAN/loopback (403)" || no "egress --block-private did not block (got $c)"
c=$($CURL '' -o /dev/null -w '%{http_code}' -x "http://127.0.0.1:$EGRESS_BLOCK" --proxytunnel "http://127.0.0.1:$ORIGIN/go.mod")
[ "$c" != 200 ] && ok "egress --block-private blocks CONNECT to LAN ($c)" || no "egress --block-private allowed CONNECT (200)"

# Reap all servers BEFORE printing so a lingering process group can't drop our
# output or signal us, then report and exit deterministically.
kill -9 "$P_ORIGIN" "$P_AUTH" "$P_EGRESS" "$P_BLOCK" 2>/dev/null
wait 2>/dev/null
rm -rf "$TMP"

printf '%s\n=== RESULT: PASS=%d FAIL=%d ===\n' "$REPORT" "$PASS" "$FAIL"
[ "$FAIL" = 0 ] && exit 0 || exit 1
