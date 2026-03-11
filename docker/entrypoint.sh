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
        # OAuth mode: write auth.json with access_token and id_token ONLY.
        # refresh_token is intentionally omitted — the Auth Proxy holds it on the host.
        # When Codex CLI needs to refresh, it calls CODEX_REFRESH_TOKEN_URL_OVERRIDE
        # which points to the proxy's /oauth/token endpoint. The proxy performs the
        # actual refresh using the host's refresh_token and returns the new access_token
        # without ever exposing the refresh_token to the container.
        #
        # Use $HOME so the correct directory is used regardless of which uid the
        # container runs as (codex-dock --user flag).  Falls back to /home/codex
        # when HOME is unset (image default behaviour).
        CODEX_HOME="${HOME:-/home/codex}"
        mkdir -p "${CODEX_HOME}/.codex"
        python3 -c "
import sys, json
d = json.load(open('/dev/stdin'))
out = {
    'auth_mode': 'chatgpt',
    'OPENAI_API_KEY': None,
    'tokens': {
        'id_token':      d.get('oauth_id_token', ''),
        'access_token':  d.get('oauth_access_token', ''),
        'refresh_token': '',
        'account_id':    d.get('oauth_account_id', ''),
    },
    'last_refresh': d.get('oauth_last_refresh', ''),
}
print(json.dumps(out))
" <<< "$RESPONSE" > "${CODEX_HOME}/.codex/auth.json"
        chmod 600 "${CODEX_HOME}/.codex/auth.json"

        # Write Codex CLI config so chatgpt.com/backend-api calls go through the proxy.
        # chatgpt_base_url overrides the default https://chatgpt.com/backend-api/ endpoint
        # used by Codex CLI for rate-limit and account-info requests (ChatGPT auth mode only).
        mkdir -p "${CODEX_HOME}/.config/codex"
        cat > "${CODEX_HOME}/.config/codex/config.toml" <<EOF
chatgpt_base_url = "${CODEX_AUTH_PROXY_URL}/chatgpt/"
EOF
        chmod 600 "${CODEX_HOME}/.config/codex/config.toml"

        log "OAuth credentials acquired (access_token is a placeholder; proxy injects real credentials on outbound API requests)."
    else
        # API key mode: extract api_key and set as environment variable
        API_KEY=$(echo "$RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin)['api_key'])" 2>/dev/null || true)

        if [[ -z "$API_KEY" ]]; then
            log "ERROR: Auth Proxy returned neither oauth_access_token nor api_key"
            exit 1
        fi

        export OPENAI_API_KEY="$API_KEY"
        log "Placeholder key acquired (real API key injected by proxy on outbound requests)."
    fi

    # Clear the temporary token from environment for security
    unset CODEX_TOKEN
    unset CODEX_AUTH_PROXY_URL
fi

# ── Build Codex arguments ───────────────────────────────────────────────────
CODEX_ARGS=()

# CODEX_APPROVAL_MODE controls how Codex CLI asks for approval.
# Values: suggest (default), auto-edit, full-auto, danger
case "${CODEX_APPROVAL_MODE:-suggest}" in
    auto-edit)
        CODEX_ARGS+=("--ask-for-approval" "unless-allow-listed")
        ;;
    full-auto)
        CODEX_ARGS+=("--ask-for-approval" "never")
        ;;
    danger)
        # Docker container isolation provides the safety boundary.
        CODEX_ARGS+=("--dangerously-bypass-approvals-and-sandbox")
        ;;
    suggest|*)
        # Default: interactive mode — no extra flags needed.
        ;;
esac

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
