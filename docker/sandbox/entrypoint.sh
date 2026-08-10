#!/bin/bash
# codex-dock secure entrypoint
# Fetches short-lived credentials from the Auth Proxy and launches the selected
# agent (Codex CLI or Claude Code), or drops into an interactive shell.
#
# Agent selection is driven by DOCK_AGENT:
#   ""       → interactive shell (auth configured, no agent auto-launched)
#   "codex"  → launch Codex CLI
#   "claude" → launch Claude Code
#
# The interactive shell binary is driven by DOCK_SHELL ("bash", the default, or
# "zsh"); see --shell-type / default_shell_type in the CLI.
#
# When DOCK_AGENT is empty and DOCK_EXEC_CMD is set (--exec), that command runs
# once instead of the interactive shell. An agent, when selected, always takes
# priority over DOCK_EXEC_CMD.
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

        # Write Codex CLI config so BOTH the Responses API (model requests) and
        # chatgpt.com/backend-api calls route through the Auth Proxy.
        #
        # openai_base_url is what actually redirects model traffic. Codex CLI does
        # NOT read the OPENAI_BASE_URL environment variable (it is deprecated and
        # ignored — openai/codex#16719); the Responses API endpoint is resolved
        # solely from this config key (codex-rs model-provider-info `to_api_provider`).
        # Without it, ChatGPT-OAuth mode connects directly to
        # https://chatgpt.com/backend-api/codex and sends the placeholder cdx-xxxx
        # token as a bearer, yielding "401 Could not parse your authentication token".
        # chatgpt_base_url only affects the ChatGPT login/backend-api flow, not the
        # model requests, so it is insufficient on its own.
        cat > "${CODEX_HOME}/.codex/config.toml" <<EOF
openai_base_url = "${CODEX_AUTH_PROXY_URL}/v1"
chatgpt_base_url = "${CODEX_AUTH_PROXY_URL}/chatgpt/"

[features]
apps = false
EOF
        chmod 600 "${CODEX_HOME}/.codex/config.toml"

        log "Codex OAuth credentials acquired (access_token is a placeholder; proxy injects real credentials on outbound API requests)."
    else
        # API key mode: extract api_key and set as environment variable
        API_KEY=$(echo "$RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin)['api_key'])" 2>/dev/null || true)

        if [[ -z "$API_KEY" ]]; then
            log "ERROR: Auth Proxy returned neither oauth_access_token nor api_key"
            exit 1
        fi

        export OPENAI_API_KEY="$API_KEY"

        # Route the Responses API through the Auth Proxy. As in OAuth mode, Codex CLI
        # only honours the openai_base_url config key, never OPENAI_BASE_URL. Without
        # this the CLI would call api.openai.com directly with the placeholder key,
        # bypassing the proxy's credential injection and returning 401.
        CODEX_HOME="$(resolve_codex_home)"
        export HOME="${CODEX_HOME}"
        mkdir -p "${CODEX_HOME}/.codex"
        cat > "${CODEX_HOME}/.codex/config.toml" <<EOF
openai_base_url = "${CODEX_AUTH_PROXY_URL}/v1"

[features]
apps = false
EOF
        chmod 600 "${CODEX_HOME}/.codex/config.toml"

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
# Always uses bash regardless of DOCK_SHELL: the auto-launch trick relies on
# PROMPT_COMMAND, which is bash-specific (zsh has no equivalent hook wired up
# here). DOCK_SHELL only selects the shell for the no-agent interactive case
# below and the --shell debug mode.
launch_interactive() {
    local init="$1"
    export PROMPT_COMMAND="unset PROMPT_COMMAND; ${init}"
    exec bash -i
}

# resolve_shell prints the shell binary to use for the no-agent interactive
# shell and --exec: DOCK_SHELL if it exists in the image, otherwise bash.
# DOCK_SHELL can come from an explicit --shell-type or be auto-detected from
# the host's $SHELL (see cmd/run.go), so it may name a shell this particular
# image never installed; falling back here keeps that auto-detection from
# turning into a hard failure on images that only ship bash.
resolve_shell() {
    local requested="${DOCK_SHELL:-bash}"
    if command -v "$requested" >/dev/null 2>&1; then
        echo "$requested"
        return
    fi
    if [[ "$requested" != "bash" ]]; then
        log "WARNING: DOCK_SHELL='${requested}' not found in this image; falling back to bash."
    fi
    echo "bash"
}

# zsh starts zsh-newuser-install when an interactive shell has no user startup
# files. This is common when a numeric host UID uses the writable fallback HOME.
# Create an empty .zshrc only when none exists; custom and bind-mounted dotfiles
# remain untouched.
ensure_zsh_startup_file() {
    local shell_bin="$1"
    [[ "${shell_bin}" == "zsh" ]] || return 0

    local startup_dir="${ZDOTDIR:-${HOME}}"
    if [[ ! -e "${startup_dir}/.zshrc" ]]; then
        mkdir -p "${startup_dir}"
        : > "${startup_dir}/.zshrc"
    fi
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
        SHELL_BIN="$(resolve_shell)"
        if [[ -n "${DOCK_EXEC_CMD:-}" ]]; then
            log "No agent selected; running command: ${DOCK_EXEC_CMD}"
            exec "${SHELL_BIN}" -c "${DOCK_EXEC_CMD}"
        fi
        ensure_zsh_startup_file "${SHELL_BIN}"
        log "No agent selected; starting interactive shell (codex and claude are available on PATH)."
        exec "${SHELL_BIN}" -i
        ;;
    *)
        log "ERROR: unknown DOCK_AGENT '${DOCK_AGENT}' (expected: codex, claude, or empty)"
        exit 1
        ;;
esac
