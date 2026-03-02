#!/bin/bash
# codex-dock secure entrypoint
# Fetches a short-lived token from Auth Proxy and launches Codex CLI.
set -euo pipefail

log() {
    echo "[codex-dock] $*" >&2
}

# ── Package installation ────────────────────────────────────────────────────
if [[ -n "${CODEX_INSTALL_SCRIPT:-}" ]]; then
    log "Installing packages..."
    bash -c "$CODEX_INSTALL_SCRIPT"
fi

# ── Auth token acquisition ──────────────────────────────────────────────────
if [[ -n "${CODEX_AUTH_PROXY_URL:-}" && -n "${CODEX_TOKEN:-}" ]]; then
    log "Fetching credentials from Auth Proxy..."

    RESPONSE=$(curl -sf \
        -H "X-Codex-Token: ${CODEX_TOKEN}" \
        "${CODEX_AUTH_PROXY_URL}/token") || {
        log "ERROR: Failed to fetch credentials from Auth Proxy at ${CODEX_AUTH_PROXY_URL}"
        exit 1
    }

    # Detect OAuth mode: proxy returns oauth_access_token instead of api_key
    OAUTH_TOKEN=$(echo "$RESPONSE" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('oauth_access_token',''))" 2>/dev/null || true)

    if [[ -n "$OAUTH_TOKEN" ]]; then
        # OAuth mode: create a synthetic auth.json with access_token only.
        # The refresh_token is intentionally omitted — it stays on the host.
        # This satisfies F-AUTH-01: auth.json is not bind-mounted from the host.
        mkdir -p /home/codex/.codex
        printf '{"access_token":"%s","token_type":"Bearer"}' "$OAUTH_TOKEN" \
            > /home/codex/.codex/auth.json
        chmod 600 /home/codex/.codex/auth.json
        log "OAuth access_token acquired (refresh_token remains on host)."
    else
        # API key mode: extract api_key and set as environment variable
        API_KEY=$(echo "$RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin)['api_key'])" 2>/dev/null || true)

        if [[ -z "$API_KEY" ]]; then
            log "ERROR: Auth Proxy returned neither oauth_access_token nor api_key"
            exit 1
        fi

        export OPENAI_API_KEY="$API_KEY"
        log "API key acquired successfully."
    fi

    # Clear the temporary token from environment for security
    unset CODEX_TOKEN
    unset CODEX_AUTH_PROXY_URL
fi

# ── Build Codex arguments ───────────────────────────────────────────────────
CODEX_ARGS=()

if [[ "${CODEX_FULL_AUTO:-0}" == "1" ]]; then
    CODEX_ARGS+=("--ask-for-approval" "never")
fi

if [[ -n "${CODEX_MODEL:-}" ]]; then
    CODEX_ARGS+=("--model" "$CODEX_MODEL")
fi

# If agents.md is specified, add it
if [[ -n "${CODEX_AGENTS_MD:-}" && -f "${CODEX_AGENTS_MD}" ]]; then
    CODEX_ARGS+=("--agents-md" "$CODEX_AGENTS_MD")
fi

# Check for AGENTS.md in workspace
if [[ -f "/workspace/AGENTS.md" ]]; then
    log "Found AGENTS.md in workspace."
fi

# ── Launch Codex ────────────────────────────────────────────────────────────
log "Starting Codex CLI..."
cd /workspace

if [[ -n "${CODEX_TASK:-}" ]]; then
    log "Task: ${CODEX_TASK}"
    exec codex "${CODEX_ARGS[@]}" "$CODEX_TASK"
else
    log "Starting in interactive mode."
    exec codex "${CODEX_ARGS[@]}"
fi
