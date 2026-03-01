FROM node:22-slim

LABEL maintainer="codex-dock"
LABEL description="Secure sandbox image for Codex CLI"

# Install system dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    git \
    make \
    curl \
    python3 \
    python3-pip \
    ripgrep \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Install Codex CLI
RUN npm install -g @openai/codex

# Create non-root user (uid:1000)
RUN useradd -m -u 1000 -s /bin/bash codex

# Create workspace directory
RUN mkdir -p /workspace && chown codex:codex /workspace

# Copy entrypoint
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# Switch to non-root user
USER codex

WORKDIR /workspace

ENTRYPOINT ["/entrypoint.sh"]
