# Installation

[日本語](../installation.md) | **English**

| Current state | Next step |
|---|---|
| New Ubuntu 26.04 host | Follow steps 1–4 below |
| Execution host already prepared | [Choose credentials and start a worker](getting-started.md#2-prepare-credentials) |
| Updating an installed node | [Update procedure](../setup.md#更新する) (Japanese) |
| Other Linux distributions | [Manual installation](../kata.md) (Japanese) |

## 1. Requirements

| Item | Requirement |
|---|---|
| OS and CPU | Ubuntu 26.04, Linux x86_64 |
| Host features | systemd, KVM (`/dev/kvm`), cgroup v2 with `cpu memory pids` |
| Account | A normal user with `sudo` access |
| Build tools | Git, Make, Go 1.24.7 or later, uv (manages Python 3.12) |
| Memory guidance | At least 8 GiB for the default two-worker configuration; allow extra capacity for builds |
| Network | Access to source, pinned runtime archives, OS packages and container images |
| WSL or a VM host | All requirements above, plus nested virtualization |

- Operator group membership grants root-equivalent host management access. Choose trusted users only.

## 2. Get tools and source

**Run in a normal user's shell.**

```bash
sudo apt-get update
sudo apt-get install -y git make curl ca-certificates golang-go

curl -LsSf https://astral.sh/uv/install.sh | sh
export PATH="$HOME/.local/bin:$PATH"
uv python install 3.12

go version
uv --version
uv run --no-project --no-config --managed-python --python 3.12 python --version
test -c /dev/kvm && echo 'KVM device found'
cat /sys/fs/cgroup/cgroup.controllers

git clone https://github.com/pacificbelt30/sandbox-codex.git
cd sandbox-codex
```

- Check the reported versions: Go 1.24.7 or later and uv-managed Python 3.12.
- Install uv using [Astral’s official instructions](https://docs.astral.sh/uv/getting-started/installation/); [uv manages Python](https://docs.astral.sh/uv/guides/install-python/).
- `install.sh` selects Python through uv and passes the same absolute interpreter path to the plan and privileged application. The OS Python is retained.
- Python distributions come from uv-managed [python-build-standalone](https://docs.astral.sh/uv/concepts/python-versions/#managed-python-distributions).
- Check that the controller list includes `cpu memory pids`.
- Missing KVM: meet the [host requirements](../kata.md#必要なホスト) (Japanese) before continuing.
- Retain this checkout until initial setup completes; the first proxy image build also uses the source.

## 3. Review the plan and install

**Run from the cloned `sandbox-codex` directory.**

```bash
./scripts/install.sh
./scripts/install.sh --apply
```

**Example plan output (excerpt)**

```text
Plan: verified Ubuntu 26.04 amd64 node; Kata 4.2.0 / Dragonball / containerd 2.4.2 / BuildKit 0.34.0 / runc 1.4.3
Bundle: /home/user/sandbox-codex/dist/node-bundle (version <version>); operator: user; cache: /var/cache/codex-dock-setup; offline: no
```

- `<version>`, paths and account names vary by installation.
- Plan only: the final line includes `No host changes. Add --apply as root.`
- Successful application: the final line starts with `Node ready.` Log in again, then verify below.

| Command or option | Behavior |
|---|---|
| No options | Build matching CLI/helpers, verify the bundle and display the installation plan |
| `--apply` | Apply through `sudo` after the plan succeeds, forwarding the selected Python’s absolute path |
| No operator option | Register the invoking normal user |
| `--operator USER` | Register another existing normal user; required when invoking the script as root |

| Installed component | Main location or name |
|---|---|
| CLI | `/usr/local/bin/oaiso` |
| Kata, containerd, BuildKit and runc | Managed assets under `/opt/codex-dock/releases/` |
| Services | `codex-dock-node`, `codex-dock-containerd`, `codex-dock-buildkit` |
| Operator group | `codex-dock` |
| Bash, Zsh and Fish completion | Under `/usr/local/share/` |

- Existing Docker and default containerd packages are retained.
- Network setup, verification and recovery: [setup details](../setup.md) (Japanese).

## 4. Log in again and verify

1. Log out, then log in again as the registered normal user.
2. Run:

```bash
command -v oaiso
oaiso --version
id -nG
oaiso runtime-check
```

**Example CLI and group output**

```text
$ command -v oaiso
/usr/local/bin/oaiso
$ oaiso --version
oaiso version <version>
$ id -nG
user sudo codex-dock
```

- `$` marks an input line; copy executable commands from the `bash` block above.
- Versions, account names and other groups vary. Check for membership in `codex-dock`.
- `runtime-check`: see [all successful checks and their meaning](getting-started.md#1-check-the-host).

| Check | Expected result |
|---|---|
| CLI | Command path and version are displayed |
| Group | `id -nG` includes `codex-dock` |
| Runtime | `runtime-check` succeeds |

## Next: credentials and first worker

1. [Choose credentials](getting-started.md#2-prepare-credentials): independent profile or legacy host integration.
2. From the source root, run `oaiso init`; use `oaiso init --agent claude` for Claude Code.
3. From your project, run `oaiso codex` or `oaiso run --agent claude`.

- Normal `init` and `run` operations do not require `sudo`.
- Real API keys and OAuth tokens stay in the Auth Proxy; work VMs receive short-lived placeholders.
- General egress defaults to denial. [Allow destinations](../proxy-policy.md) (Japanese) when Git or package downloads are needed.

## What `--bundle` means

- A directory containing the prebuilt `oaiso` CLI, worker guard, I/O logger and `bundle.json` (version and SHA-256 checksums).
- Normal installation creates it in `dist/node-bundle/`; you do not need to specify the option.
- Use `--bundle PATH` to reuse a build or bring it from another host. The destination host needs uv/Python, but not Go/Make.
- It excludes Kata archives, container images, projects and credentials. Offline installation also needs prepared archives and Python.

```bash
./scripts/install.sh --bundle /path/to/node-bundle
./scripts/install.sh --bundle /path/to/node-bundle --apply
```

## Other installation paths

| Method | Scope and reference |
|---|---|
| Manual host installation | [Kata host setup](../kata.md) (Japanese) |
| Prebuilt CLI/helper directory | [Bundle contents](../setup.md#ビルド済みバンドルとは) (Japanese); `--bundle PATH` skips Go/Make on this host |
| Offline installation | [Offline setup](../setup.md#オフライン配置) (Japanese) |
| Build and install CLI, helpers and completion | `make build` / `sudo make install`; excludes Kata and host services |
| Updates and rollback | [Update and recovery](../setup.md#更新する) (Japanese) |
