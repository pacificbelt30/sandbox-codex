#!/usr/bin/env bash
set -euo pipefail

# Minimal firewall rules for codex-dock with standalone auth-proxy.
# - Requires Linux + iptables + Docker.
# - Safely skips network-specific rules if dock-net / dock-net-proxy are absent.

DOCK_NET_BRIDGE="${DOCK_NET_BRIDGE:-dock-net0}"
PROXY_NET_BRIDGE="${PROXY_NET_BRIDGE:-dock-net-proxy0}"
PROXY_PORT="${PROXY_PORT:-18080}"

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "error: required command not found: $1" >&2
    exit 1
  }
}

iface_exists() {
  ip link show "$1" >/dev/null 2>&1
}

ensure_chain() {
  iptables -N CODEX-DOCK 2>/dev/null || true
  iptables -C DOCKER-USER -j CODEX-DOCK 2>/dev/null || iptables -A DOCKER-USER -j CODEX-DOCK
}

ensure_rule() {
  local chain="$1"; shift
  iptables -C "$chain" "$@" 2>/dev/null || iptables -A "$chain" "$@"
}

require_cmd iptables
require_cmd ip

if [[ "$(id -u)" -ne 0 ]]; then
  echo "error: run as root (e.g. sudo $0)" >&2
  exit 1
fi

ensure_chain

if iface_exists "$DOCK_NET_BRIDGE"; then
  # Keep established flows.
  ensure_rule CODEX-DOCK -i "$DOCK_NET_BRIDGE" -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT

  # Block worker -> host private ranges by default.
  ensure_rule CODEX-DOCK -i "$DOCK_NET_BRIDGE" -d 10.0.0.0/8 -j DROP
  ensure_rule CODEX-DOCK -i "$DOCK_NET_BRIDGE" -d 172.16.0.0/12 -j DROP
  ensure_rule CODEX-DOCK -i "$DOCK_NET_BRIDGE" -d 192.168.0.0/16 -j DROP
  ensure_rule CODEX-DOCK -i "$DOCK_NET_BRIDGE" -d 169.254.0.0/16 -j DROP

  if iface_exists "$PROXY_NET_BRIDGE"; then
    # Allow worker -> proxy only.
    ensure_rule DOCKER-USER -i "$DOCK_NET_BRIDGE" -o "$PROXY_NET_BRIDGE" -p tcp --dport "$PROXY_PORT" -j ACCEPT
    ensure_rule DOCKER-USER -i "$PROXY_NET_BRIDGE" -o "$DOCK_NET_BRIDGE" -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT
  else
    echo "warn: $PROXY_NET_BRIDGE not found; skip cross-network allow rules"
  fi

  # Final gate for worker network.
  ensure_rule DOCKER-USER -i "$DOCK_NET_BRIDGE" -j CODEX-DOCK
else
  echo "warn: $DOCK_NET_BRIDGE not found; skip dock-net specific rules"
  echo "hint: create it first with: codex-dock network create"
fi

echo "Applied minimal iptables rules for codex-dock standalone auth-proxy."
iptables -S DOCKER-USER
