# Build images

[日本語](../../commands/build.md) | **English**

`oaiso build` uses standalone BuildKit and imports OCI output into the worker's containerd namespace.
Dockerfile format is supported without Docker Engine or Docker CLI.

## Commands

```bash
oaiso build
oaiso build --template list
oaiso build --template pwn
oaiso build -f ./Dockerfile --tag my-codex:v1
oaiso run --image my-codex:v1
```

| Option | Purpose |
|---|---|
| `--tag`, `-t` | Image tag; normally `codex-dock:latest`, or the selected template's tag |
| `--dockerfile`, `-f` | Dockerfile path |
| `--template`, `-T` | Template name, or `list` |
| `--print-entrypoint` | Print embedded entrypoint.sh and exit without building |

The built-in worker pins Codex CLI with `CODEX_CLI_VERSION` in its Dockerfile. To test another exact version, build a separate OCI image in the worker's containerd namespace:

```bash
CODEX_DOCK_CODEX_VERSION=0.163.0 oaiso build --template plain --tag codex-dock:codex-0.163.0
oaiso image check codex-dock:codex-0.163.0 --agent codex
oaiso run --image codex-dock:codex-0.163.0 --agent codex
```

`--template` and `-f` are mutually exclusive.
Missing images are built from the corresponding template or default Dockerfile.
Build arbitrary custom tags explicitly before using them.
With a `[build]` section in the project's `codex-dock.toml`, `build`, `init`, and `run` use the same build source. Missing images are built after a fresh clone. See [project settings](../configuration.md#project-settings).

Claude Code is pinned with `CLAUDE_CLI_VERSION=2.1.306`. Use `CODEX_DOCK_CLAUDE_VERSION` to override it for a build. See [Claude Code](../claude-code.md) for image/proxy updates and verification.

## Custom image

First build the base image with `oaiso build`.
Create this Dockerfile in a new working directory:

```dockerfile
FROM codex-dock:latest
USER root
RUN apt-get update \
    && apt-get install -y --no-install-recommends jq \
    && rm -rf /var/lib/apt/lists/*
USER codex
```

Then build and run it from that directory:

```bash
oaiso build -f ./Dockerfile --tag my-codex:jq
oaiso run --image my-codex:jq -- jq --version
```

The image inherits the agents and auth entrypoint.
Root is used for the build step; the Kata worker runs with a resolved non-root UID and verified restrictions.
Do not include credentials or management sockets in images.

## Independent base image

Install the selected agent and entrypoint dependencies, using `docker/sandbox/Dockerfile` as the reference.
Get the matching entrypoint from your CLI:

```bash
oaiso build --print-entrypoint > entrypoint.sh
```

After creating the non-root user and installing dependencies, include:

```dockerfile
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
USER codex
WORKDIR /workspace
ENTRYPOINT ["/entrypoint.sh"]
```

Refresh the entrypoint when updating the CLI. Custom images pass the same Kata startup gate and need their own compatibility checks.

## User templates

Place a Dockerfile at `~/.config/codex-dock/templates/<name>/Dockerfile`.
User templates override built-ins with the same name.

```bash
mkdir -p ~/.config/codex-dock/templates/mytool
cp Dockerfile ~/.config/codex-dock/templates/mytool/Dockerfile
oaiso build --template mytool
oaiso run --template mytool
```

`plain` uses `codex-dock:latest`; other names use `codex-dock:<name>`.
`make install-templates` copies built-ins without overwriting existing files.
See [auto_build](../configuration.md#auto_build).

## Build context

With explicit `-f`, the context is the current directory.
Otherwise the search order is `./Dockerfile`, `./docker/sandbox/Dockerfile`, `./docker/Dockerfile`, then `~/.config/codex-dock/Dockerfile`.
The selected Dockerfile directory becomes the context. Missing config defaults are generated.
Template builds create a temporary context with required entrypoint files.
Proxy images use [proxy build](proxy.md).

## Builder connection

The default address is `unix:///run/codex-dock-buildkit/buildkitd.sock`; override with `BUILDKIT_HOST`.
Use the dedicated containerd socket and namespace for BuildKit's containerd worker.
Proxy environment variables are passed as build args.
`CODEX_PROXY_CERT` is supplied as the `proxy_ca` secret, used only during RUN in built-in Dockerfiles.
Configure BuildKit daemon registry proxy/CA access separately.
