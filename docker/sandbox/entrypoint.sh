#!/bin/bash
# codex-dock secure entrypoint
# Fetches short-lived credentials from the Auth Proxy and launches the selected
# agent (Codex CLI or Claude Code), or drops into an interactive shell.
#
# Agent selection is driven by DOCK_AGENT:
#   ""       → interactive shell (auth configured, no agent auto-launched)
#   "codex"  → launch Codex CLI
#   "claude" → launch Claude Code
set -euo pipefail

log() {
    echo "[codex-dock] $*" >&2
}

DOCK_AGENT="${DOCK_AGENT:-}"

resolve_codex_home() {
    local candidate
    for candidate in "${HOME:-}" "/var/tmp/codex-home" "/tmp/codex-home"; do
        [[ -z "$candidate" ]] && continue
        if mkdir -p "$candidate" 2>/dev/null && [[ -w "$candidate" ]]; then
            echo "$candidate"
            return 0
        fi
    done
    echo "/tmp"
}

# ── Package installation ────────────────────────────────────────────────────
if [[ -n "${CODEX_INSTALL_SCRIPT:-}" ]]; then
    log "Installing packages..."
    bash -c "$CODEX_INSTALL_SCRIPT"
fi

# ── Codex (OpenAI) auth acquisition ───────────────────────────────────────────
# Runs whenever the proxy issued an OpenAI/Codex token (agent=codex or shell).
if [[ -n "${CODEX_AUTH_PROXY_URL:-}" && -n "${CODEX_TOKEN:-}" ]]; then
    log "Fetching Codex credentials from Auth Proxy (${CODEX_AUTH_PROXY_URL})..."

    # The worker reaches the proxy over its dedicated Internal Docker network via
    # the embedded DNS name (codex-auth-proxy). NO_PROXY excludes that host so this
    # request hits the proxy directly instead of looping through the forward proxy.
    fetch_token() {
        local endpoint="$1"
        curl -sf --connect-timeout 3 --max-time 10 \
            -H "X-Codex-Token: ${CODEX_TOKEN}" \
            "${endpoint}/token"
    }

    RESPONSE=$(fetch_token "${CODEX_AUTH_PROXY_URL}") || {
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
        CODEX_HOME="$(resolve_codex_home)"
        export HOME="${CODEX_HOME}"
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
        mkdir -p "${CODEX_HOME}/.config/codex"
        cat > "${CODEX_HOME}/.config/codex/config.toml" <<EOF
chatgpt_base_url = "${CODEX_AUTH_PROXY_URL}/chatgpt/"
EOF
        chmod 600 "${CODEX_HOME}/.config/codex/config.toml"

        log "Codex OAuth credentials acquired (access_token is a placeholder; proxy injects real credentials on outbound API requests)."
    else
        # API key mode: extract api_key and set as environment variable
        API_KEY=$(echo "$RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin)['api_key'])" 2>/dev/null || true)

        if [[ -z "$API_KEY" ]]; then
            log "ERROR: Auth Proxy returned neither oauth_access_token nor api_key"
            exit 1
        fi

        export OPENAI_API_KEY="$API_KEY"
        log "Codex placeholder key acquired (real API key injected by proxy on outbound requests)."
    fi

    # Clear the temporary token from environment for security
    unset CODEX_TOKEN
    unset CODEX_AUTH_PROXY_URL
fi

# ── Claude Code (Anthropic) auth setup ────────────────────────────────────────
# Claude Code is fully env-driven: ANTHROPIC_BASE_URL points at the proxy's
# /anthropic route and ANTHROPIC_API_KEY carries a placeholder. The proxy
# overwrites the credential with the host's real API key or OAuth bearer token
# on every outbound request, so the real Anthropic credential never reaches the
# container (mirrors the Codex flow).
if [[ -n "${ANTHROPIC_BASE_URL:-}" ]]; then
    CLAUDE_HOME="$(resolve_codex_home)"
    export HOME="${CLAUDE_HOME}"
    # Skip first-run onboarding/theme prompts so the agent starts unattended.
    if [[ ! -f "${CLAUDE_HOME}/.claude.json" ]]; then
        cat > "${CLAUDE_HOME}/.claude.json" <<'EOF'
{"hasCompletedOnboarding": true}
EOF
        chmod 600 "${CLAUDE_HOME}/.claude.json"
    fi
    log "Claude Code auth configured via proxy (${ANTHROPIC_BASE_URL}); real credential injected by proxy on outbound requests."
fi

# ── Build agent arguments ─────────────────────────────────────────────────────
# CODEX_APPROVAL_MODE: suggest (default), auto-edit, full-auto, danger
build_codex_args() {
    CODEX_ARGS=()
    case "${CODEX_APPROVAL_MODE:-suggest}" in
        auto-edit) CODEX_ARGS+=("--ask-for-approval" "unless-allow-listed") ;;
        full-auto) CODEX_ARGS+=("--ask-for-approval" "never") ;;
        danger)    CODEX_ARGS+=("--dangerously-bypass-approvals-and-sandbox") ;;
        suggest|*) ;;
    esac
    if [[ -n "${CODEX_MODEL:-}" ]]; then
        CODEX_ARGS+=("--model" "$CODEX_MODEL")
    fi
    if [[ -n "${CODEX_AGENTS_MD:-}" && -f "${CODEX_AGENTS_MD}" ]]; then
        CODEX_ARGS+=("--agents-md" "$CODEX_AGENTS_MD")
    fi
}

build_claude_args() {
    CLAUDE_ARGS=()
    case "${CODEX_APPROVAL_MODE:-suggest}" in
        auto-edit)        CLAUDE_ARGS+=("--permission-mode" "acceptEdits") ;;
        full-auto|danger) CLAUDE_ARGS+=("--dangerously-skip-permissions") ;;
        suggest|*)        ;;
    esac
    if [[ -n "${CODEX_MODEL:-}" ]]; then
        CLAUDE_ARGS+=("--model" "$CODEX_MODEL")
    fi
}

# launch_interactive launches an agent after bash job-control is ready so that
# Ctrl+Z suspends the agent and returns to an in-container bash prompt.
launch_interactive() {
    local init="$1"
    export PROMPT_COMMAND="unset PROMPT_COMMAND; ${init}"
    exec bash -i
}

if [[ -f "/workspace/AGENTS.md" ]]; then
    log "Found AGENTS.md in workspace."
fi

cd /workspace

# Give Docker networking/proxy sidecars a brief moment to settle before the
# first agent request. Reduces flaky startup failures right after container boot.
sleep 1

# ── Launch the selected agent ─────────────────────────────────────────────────
case "${DOCK_AGENT}" in
    codex)
        build_codex_args
        log "Starting Codex CLI..."
        if [[ -n "${CODEX_TASK:-}" ]]; then
            log "Task: ${CODEX_TASK}"
            exec codex "${CODEX_ARGS[@]}" "$CODEX_TASK"
        fi
        INIT="codex"
        for arg in "${CODEX_ARGS[@]}"; do INIT+=" $(printf '%q' "$arg")"; done
        launch_interactive "$INIT"
        ;;
    claude)
        build_claude_args
        log "Starting Claude Code..."
        if [[ -n "${CODEX_TASK:-}" ]]; then
            log "Task: ${CODEX_TASK}"
            exec claude "${CLAUDE_ARGS[@]}" "$CODEX_TASK"
        fi
        INIT="claude"
        for arg in "${CLAUDE_ARGS[@]}"; do INIT+=" $(printf '%q' "$arg")"; done
        launch_interactive "$INIT"
        ;;
    ""|shell)
        log "No agent selected; starting interactive shell (codex and claude are available on PATH)."
        exec bash -i
        ;;
    *)
        log "ERROR: unknown DOCK_AGENT '${DOCK_AGENT}' (expected: codex, claude, or empty)"
        exit 1
        ;;
esac
