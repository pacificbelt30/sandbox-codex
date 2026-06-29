FROM codex-dock:latest

LABEL description="codex-dock pwn/RE template: pwntools, pwndbg, radare2, etc."

USER root

# System-level RE / binary-analysis tools
RUN apt-get update && apt-get install -y --no-install-recommends \
    gdb \
    nasm \
    strace \
    ltrace \
    binwalk \
    file \
    binutils \
    libc6-dbg \
    libssl-dev \
    libffi-dev \
    python3-dev \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

# Python pwn/RE libraries
RUN pip3 install --break-system-packages --no-cache-dir \
    pwntools \
    ptrlib \
    ropper

# radare2 (latest from GitHub releases — apt version is typically very old)
RUN curl -Ls https://github.com/radareorg/radare2/releases/latest/download/radare2_amd64.deb \
      -o /tmp/r2.deb \
    && dpkg -i /tmp/r2.deb || apt-get install -yf --no-install-recommends \
    && rm -f /tmp/r2.deb

# pwndbg (GDB plugin)
RUN git clone --depth 1 https://github.com/pwndbg/pwndbg.git /opt/pwndbg \
    && cd /opt/pwndbg \
    && ./setup.sh \
    && echo "source /opt/pwndbg/gdbinit.py" >> /home/codex/.gdbinit \
    && chown codex:codex /home/codex/.gdbinit

USER codex
WORKDIR /workspace
