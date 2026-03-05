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
        # OAuth mode: reconstruct auth.json from all fields provided by the Auth Proxy.
        # WARNING: This includes refresh_token. The container has equivalent credentials
        # to the host for the duration of its lifetime. See doc/auth-proxy.md.
        ID_TOKEN=$(echo "$RESPONSE" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('oauth_id_token',''))" 2>/dev/null || true)
        REFRESH_TOKEN=$(echo "$RESPONSE" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('oauth_refresh_token',''))" 2>/dev/null || true)
        ACCOUNT_ID=$(echo "$RESPONSE" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('oauth_account_id',''))" 2>/dev/null || true)
        LAST_REFRESH=$(echo "$RESPONSE" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('oauth_last_refresh',''))" 2>/dev/null || true)
        mkdir -p /home/codex/.codex
        python3 -c "
import sys, json
d = json.load(open('/dev/stdin'))
out = {
    'auth_mode': 'chatgpt',
    'OPENAI_API_KEY': None,
    'tokens': {
        'id_token':      d.get('oauth_id_token', ''),
        'access_token':  d.get('oauth_access_token', ''),
        'refresh_token': d.get('oauth_refresh_token', ''),
        'account_id':    d.get('oauth_account_id', ''),
    },
    'last_refresh': d.get('oauth_last_refresh', ''),
}
print(json.dumps(out))
" <<< "$RESPONSE" > /home/codex/.codex/auth.json
        chmod 600 /home/codex/.codex/auth.json
        log "OAuth credentials acquired (all token fields written to auth.json)."
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
    log "Starting in interactive mode (bash + codex with job control)."
    # Build the codex invocation string.
    CODEX_INIT="codex"
    for arg in "${CODEX_ARGS[@]}"; do
        CODEX_INIT+=" $(printf '%q' "$arg")"
    done
    # Use PROMPT_COMMAND to launch codex *after* bash has fully initialised
    # its job-control machinery.  --init-file runs too early (before job
    # control is ready), so Ctrl+Z would not work there.
    # PROMPT_COMMAND fires just before the first prompt: bash is the session
    # leader, ISIG is active, and Ctrl+Z properly suspends codex, returning
    # to the bash prompt inside the container.  `fg` resumes codex.
    export PROMPT_COMMAND="unset PROMPT_COMMAND; $CODEX_INIT"
    exec bash -i
fi
