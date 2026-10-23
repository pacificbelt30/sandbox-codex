# Prepare development tools

[日本語](../../development/toolchain.md) | **English**

| Purpose | Go, Make and Git |
|---|---|
| [Install a prebuilt release](../installation.md) | Not required; the installer also prepares missing uv/Python |
| Build the CLI, node and helpers from source | Required; follow this page |
| Build a product embedding the Go API | Go is required in the build environment, which can be separate from the execution host |

## Required tools and official instructions

| Tool | Purpose or requirement | Official instructions or downloads |
|---|---|---|
| Go | `go.mod` requires at least 1.24.7; builds the CLI, node and helpers | [Linux installation](https://go.dev/doc/install), [downloads and SHA-256](https://go.dev/dl/) |
| GNU Make | Build and test targets | [Official downloads and documentation](https://www.gnu.org/software/make/) |
| Git | Source checkout and version control | [Linux installation](https://git-scm.com/install/linux) |
| curl | Download release assets | [Official download guide](https://curl.se/download.html) |
| uv | Isolated host Python and dependency management | [Installation](https://docs.astral.sh/uv/getting-started/installation/) |
| Python 3.12 | Setup, validation and documentation scripts | [Install Python with uv](https://docs.astral.sh/uv/guides/install-python/) |
| C compiler | Go race tests; also builds the static PID hook during manual Kata preparation | [GCC installation](https://gcc.gnu.org/install/) |

- Python uses uv-managed distributions, rather than a source build of Python itself.
- The commands below target Ubuntu 26.04 on Linux x86_64. Go uses the official archive; Git, Make and GCC use Ubuntu packages.

## 1. Prepare basic tools

```bash
sudo apt-get update
sudo apt-get install -y curl ca-certificates git make build-essential
```

## 2. Install Go using the official Linux procedure

**Skip to step 3 if a suitable Go installation is already available.**

1. Check the stable Linux amd64 archive and SHA-256 on the [official download page](https://go.dev/dl/), then download and verify it.

    ```bash
    curl -fL -o go1.27.3.linux-amd64.tar.gz https://go.dev/dl/go1.27.3.linux-amd64.tar.gz
    echo '65efbc80fdc1fa2cd2631cad5b481777a68e8725f34e89f50fc6ac714e379b40  go1.27.3.linux-amd64.tar.gz' | sha256sum --check -
    ```

    - 1.27.3 is an example. For another version, change the filename, URL and SHA-256 together.
    - Expected: `go1.27.3.linux-amd64.tar.gz: OK`. Do not continue if verification fails.

2. After successful verification, remove the existing `/usr/local/go` tree and extract the archive as instructed by Go.

    ```bash
    sudo rm -rf /usr/local/go
    sudo tar -C /usr/local -xzf go1.27.3.linux-amd64.tar.gz
    ```

    - This replaces the toolchain under `/usr/local/go`. Do not extract over an existing Go tree.

3. Add the following line to `~/.profile` and also run it in the current shell.

    ```bash
    export PATH="/usr/local/go/bin:$PATH"
    ```

4. Verify the active installation.

    ```bash
    command -v go
    go version
    ```

    ```text
    /usr/local/go/bin/go
    go version go1.27.3 linux/amd64
    ```

    - The version should match the installed archive. If another Go is selected, check PATH ordering.

## 3. Prepare uv and Python

**Only install uv if it is missing, using its [official versioned installer](https://docs.astral.sh/uv/getting-started/installation/#standalone-installer).**

```bash
curl -LsSf https://astral.sh/uv/0.13.5/install.sh | sh
export PATH="$HOME/.local/bin:$PATH"
uv python install 3.12
uv --version
uv python find --no-project --no-config --managed-python --system 3.12
```

- uv 0.13.5 matches CI. If uv is already installed, only prepare and check Python.
- The last command should print the absolute path of a uv-managed Python 3.12 executable.
- The OS Python is retained.

## 4. Continue to build or validate

- [Build and deploy from source](../../setup.md#ソースからビルドする場合) (Japanese)
- [Code validation](../../testing.md#コードを確認する) (Japanese)
- [Documentation editing](documentation.md)
- [Create a release](../../development/releases.md) (Japanese)
