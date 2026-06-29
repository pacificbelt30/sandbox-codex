#!/usr/bin/env bash
#
# e2e-docker.sh — end-to-end check of the container-level isolation that needs a
# real Docker daemon. SKIPS cleanly (exit 0) when Docker is unavailable.
#
# What it verifies (the parts the Go unit tests / smoke test cannot):
#   - `codex-dock proxy run` brings up BOTH proxies (codex-auth-proxy +
#     codex-http-proxy) on the egress network, publishing only the admin port to
#     host loopback (data/forward ports stay internal).
#   - A worker on its per-worker Internal network can reach the auth proxy
#     (data plane) and the http proxy, but NOT the admin port.
#   - The worker has NO direct internet (Internal net, no NAT).
#   - Two workers on separate per-worker networks cannot reach each other.
#   - The http proxy with --block-private refuses private/LAN destinations.
#
# The worker side mirrors what `codex-dock run` builds (per-worker Internal net
# with both proxies attached), using a small curl image so it needs no agent or
# real API credentials. Requires: go, docker (with image pull access).
set -u

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

if ! docker version >/dev/null 2>&1; then
  echo "SKIP: no reachable Docker daemon — container-level E2E not run."
  echo "      (Run this on a host/CI with Docker to exercise worker isolation.)"
  exit 0
fi

WORKER_IMAGE="curlimages/curl:latest"
# Dedicated listener image for worker2. curlimages/curl is a stripped Alpine
# without the busybox httpd applet, so it can't host a port — use canonical
# busybox (httpd applet always present) for the listener.
LISTENER_IMAGE="busybox:latest"
EGRESS_NET="dock-net-proxy"
W1NET="dock-net-w-e2e1"; W2NET="dock-net-w-e2e2"
W1="e2e-worker-1"; W2="e2e-worker-2"; PROBE="e2e-probe"
AUTH="codex-auth-proxy"; HTTP="codex-http-proxy"
PASS=0; FAIL=0
ok(){ echo "PASS: $1"; PASS=$((PASS+1)); }
no(){ echo "FAIL: $1"; FAIL=$((FAIL+1)); }

BIN="$(mktemp -d)/codex-dock"
cleanup(){
  # Remove every container attached to the per-worker nets BEFORE removing the
  # nets. The proxies are multi-homed onto W1NET/W2NET, so the nets can't be
  # removed while the proxies are still up — drop workers + proxies first.
  docker rm -f "$W1" "$W2" "$PROBE" >/dev/null 2>&1 || true
  "$BIN" proxy rm -f >/dev/null 2>&1 || true
  docker network rm "$W1NET" "$W2NET" >/dev/null 2>&1 || true
  rm -rf "$(dirname "$BIN")"
}
trap cleanup EXIT

echo "=== build codex-dock ==="
go build -o "$BIN" . || { echo "build failed"; exit 1; }

# Worker (curl) image — pulled on the host before attaching to an Internal net.
if ! docker pull "$WORKER_IMAGE" >/dev/null 2>&1; then
  echo "SKIP: cannot pull $WORKER_IMAGE (no registry access); proxy checks only."
  WORKER_IMAGE=""
fi
# Listener image for worker2 (optional). If it can't be pulled we fall back to a
# non-listening worker2 and drop the positive-control probe.
if [ -n "$WORKER_IMAGE" ] && ! docker pull "$LISTENER_IMAGE" >/dev/null 2>&1; then
  echo "NOTE: cannot pull $LISTENER_IMAGE; worker2 will not listen (isolation test runs without positive control)."
  LISTENER_IMAGE=""
fi

echo "=== start proxies (codex-dock proxy run) ==="
"$BIN" proxy rm -f >/dev/null 2>&1 || true
if ! "$BIN" proxy run >/dev/null 2>&1; then
  echo "FAIL: codex-dock proxy run failed (proxy image build needs registry access?)"
  exit 1
fi

state(){ docker inspect -f '{{.State.Status}}' "$1" 2>/dev/null; }
[ "$(state "$AUTH")" = running ] && ok "auth proxy running"  || no "auth proxy not running"
[ "$(state "$HTTP")" = running ] && ok "http proxy running"  || no "http proxy not running"

# Admin published to host loopback; data/forward ports NOT published.
ports="$(docker inspect -f '{{json .NetworkSettings.Ports}}' "$AUTH" 2>/dev/null)$(docker inspect -f '{{json .NetworkSettings.Ports}}' "$HTTP" 2>/dev/null)"
echo "$ports" | grep -q '18081' && ok "admin port published" || no "admin port not published"
if echo "$ports" | grep -q '"18080/tcp":\[{' || echo "$ports" | grep -q '"18082/tcp":\[{'; then
  no "data/forward port unexpectedly published to host"
else
  ok "data/forward ports NOT host-published"
fi

if [ -z "$WORKER_IMAGE" ]; then
  echo; echo "=== RESULT: PASS=$PASS FAIL=$FAIL (worker phase skipped) ==="
  [ "$FAIL" = 0 ] && exit 0 || exit 1
fi

echo "=== build worker topology (mirrors codex-dock run) ==="
docker network create --internal "$W1NET" >/dev/null
docker network create --internal "$W2NET" >/dev/null
docker network connect "$W1NET" "$AUTH" >/dev/null; docker network connect "$W1NET" "$HTTP" >/dev/null
docker network connect "$W2NET" "$AUTH" >/dev/null; docker network connect "$W2NET" "$HTTP" >/dev/null
docker run -d --name "$W1" --network "$W1NET" --entrypoint sleep "$WORKER_IMAGE" 600 >/dev/null
if [ -n "$LISTENER_IMAGE" ]; then
  # Worker2 actually LISTENS on :80 (busybox httpd) so the worker<->worker
  # isolation test has a real service to (fail to) reach.
  docker run -d --name "$W2" --network "$W2NET" "$LISTENER_IMAGE" \
    sh -c 'mkdir -p /tmp/web && echo ok > /tmp/web/index.html && httpd -f -p 80 -h /tmp/web' >/dev/null
  # Probe on the SAME net as worker2 — positive control proving :80 is reachable
  # when L2 allows it, so a 000 from worker1 means isolation (not a dead port).
  docker run -d --name "$PROBE" --network "$W2NET" --entrypoint sleep "$WORKER_IMAGE" 600 >/dev/null
else
  # No listener image: worker2 just idles; isolation test runs without the
  # positive control (a 000 then can't distinguish isolation from a dead port).
  docker run -d --name "$W2" --network "$W2NET" --entrypoint sleep "$WORKER_IMAGE" 600 >/dev/null
fi

# Run curl inside a container; echoes the HTTP code (000 on failure).
wcode(){ docker exec "$1" curl -s -o /dev/null -w '%{http_code}' --max-time 6 "${@:2}" 2>/dev/null; }

# Positive controls: the worker can reach both proxies on its Internal net.
[ "$(wcode "$W1" http://$AUTH:18080/health)" = 200 ] && ok "worker reaches auth data-plane (/health 200)" || no "worker cannot reach auth data-plane"
[ "$(wcode "$W1" http://$HTTP:18082/health)" = 200 ] && ok "worker reaches http proxy (/health 200)" || no "worker cannot reach http proxy"

# Isolation: admin port NOT reachable from the worker (bound to egress IP).
c=$(wcode "$W1" http://$AUTH:18081/admin/mode); [ "$c" != 200 ] && ok "worker cannot reach admin port ($c)" || no "worker reached admin port (200)"

# Isolation: no direct internet (Internal net, no NAT).
c=$(wcode "$W1" https://1.1.1.1); [ "$c" = 000 ] && ok "worker has no direct internet (blocked)" || no "worker reached internet directly ($c)"

# Isolation: worker1 cannot reach worker2 (separate Internal nets).
W2IP=$(docker inspect -f "{{(index .NetworkSettings.Networks \"$W2NET\").IPAddress}}" "$W2" 2>/dev/null)
if [ -n "$LISTENER_IMAGE" ]; then
  # Positive control: the probe on W2NET CAN reach worker2's listener, so a 000
  # from worker1 proves L2 isolation rather than a non-listening port.
  c=$(wcode "$PROBE" "http://$W2IP:80/"); [ "$c" = 200 ] && ok "positive control: same-net probe reaches worker2 listener (200)" || no "probe could not reach worker2 listener ($c) — isolation test inconclusive"
fi
c=$(wcode "$W1" "http://$W2IP:80/"); [ "$c" = 000 ] && ok "worker1 cannot reach worker2 ($W2IP)" || no "worker1 reached worker2 ($c)"

# LAN block: http proxy refuses a private/LAN destination.
c=$(wcode "$W1" -x http://$HTTP:18082 http://10.255.255.1/); [ "$c" = 403 ] && ok "http proxy blocks private/LAN (403)" || no "http proxy did not block private dest ($c)"

echo
echo "=== RESULT: PASS=$PASS FAIL=$FAIL ==="
[ "$FAIL" = 0 ] && exit 0 || exit 1
