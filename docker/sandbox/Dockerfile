FROM node:22-slim

LABEL maintainer="codex-dock"
LABEL description="Secure sandbox image for Codex CLI and Claude Code"

# Install system dependencies
RUN --mount=type=secret,id=proxy_ca \
    if [ -r /run/secrets/proxy_ca ]; then export APT_CONFIG=/tmp/codex-dock-apt.conf; printf 'Acquire::https::CaInfo "/run/secrets/proxy_ca";\n' > "$APT_CONFIG"; fi; \
    apt-get update && apt-get install -y --no-install-recommends \
    git \
    make \
    curl \
    python3 \
    python3-pip \
    ripgrep \
    ca-certificates \
    zsh \
    && rm -rf /var/lib/apt/lists/* /tmp/codex-dock-apt.conf

# Install agent CLIs:
#   - @openai/codex          → `oaiso run --agent codex`
#   - @anthropic-ai/claude-code → `oaiso run --agent claude`
# Both are installed so a single sandbox image can run either agent.
ARG CODEX_CLI_VERSION=0.163.0
ARG CLAUDE_CLI_VERSION=2.1.306
LABEL org.codex-dock.codex-cli-version="${CODEX_CLI_VERSION}"
LABEL org.codex-dock.claude-cli-version="${CLAUDE_CLI_VERSION}"
RUN --mount=type=secret,id=proxy_ca \
    if [ -r /run/secrets/proxy_ca ]; then export NODE_EXTRA_CA_CERTS=/run/secrets/proxy_ca; fi; \
    npm install -g "@openai/codex@${CODEX_CLI_VERSION}" "@anthropic-ai/claude-code@${CLAUDE_CLI_VERSION}"

# Create non-root user (uid:1001; uid:1000 is reserved for the base image's node user)
RUN useradd -m -u 1001 -s /bin/bash codex

# Create workspace and credential directories owned by the non-root user.
#   ~/.codex/auth.json         → Codex ChatGPT subscription auth (proxy placeholder)
#   ~/.claude/.credentials.json → Claude Code subscription auth (never mounted; proxy injects)
RUN mkdir -p /workspace /home/codex/.codex /home/codex/.claude \
    && chown -R codex:codex /workspace /home/codex/.codex /home/codex/.claude

# Copy entrypoint
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# Switch to non-root user
USER codex

WORKDIR /workspace

LABEL io.codex-dock.start-hooks="1"
ENTRYPOINT ["/entrypoint.sh"]
