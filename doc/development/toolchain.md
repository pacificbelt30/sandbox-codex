# 開発用ツールを準備する

**日本語** | [English](../en/development/toolchain.md)

| 目的 | Go・Make・Git |
|---|---|
| [ビルド済み版をインストール](../installation.md) | 不要。uv・Pythonも未導入ならインストーラーが準備 |
| CLI・node・補助プログラムをソースからビルド | 必要。このページの手順を使用 |
| Go APIを組み込む製品をビルド | Goが必要。Oaisoの実行ホストとは別のビルド環境に配置可能 |

## 必要なツールと公式手順

| ツール | 用途・条件 | 公式手順・配布元 |
|---|---|---|
| Go | `go.mod` の最低版は1.24.7。CLI・node・補助プログラムのビルド | [Linux向けインストール](https://go.dev/doc/install)・[配布物とSHA-256](https://go.dev/dl/) |
| GNU Make | `make build`・テストなど | [公式配布・ドキュメント](https://www.gnu.org/software/make/) |
| Git | ソース取得・変更管理 | [Linux向けインストール](https://git-scm.com/install/linux) |
| curl | 配布物の取得 | [公式ダウンロード案内](https://curl.se/download.html) |
| uv | 分離したホストPythonと依存関係の管理 | [インストール](https://docs.astral.sh/uv/getting-started/installation/) |
| Python 3.12 | 配置・検証・ドキュメント処理 | [uvによるPythonの導入](https://docs.astral.sh/uv/guides/install-python/) |
| Cコンパイラー | Goのrace検査。Kataを手動準備する場合は静的PID hookのビルドにも使用 | [GCCの導入](https://gcc.gnu.org/install/) |

- Pythonはuv管理の配布物を使用。Python本体をソースからビルドする手順ではありません。
- 以下はUbuntu 26.04・Linux x86_64の例。Goは公式アーカイブ、Git・Make・GCCなどはUbuntuパッケージを使用します。

## 1. 基本ツールを準備する

```bash
sudo apt-get update
sudo apt-get install -y curl ca-certificates git make build-essential
```

## 2. Goを公式Linux手順で導入する

**既に必要な版のGoが使える場合は、手順3へ進んでください。**

1. [公式ダウンロード一覧](https://go.dev/dl/)でLinux amd64の安定版とSHA-256を確認し、取得・検証。

    ```bash
    curl -fL -o go1.27.3.linux-amd64.tar.gz https://go.dev/dl/go1.27.3.linux-amd64.tar.gz
    echo '65efbc80fdc1fa2cd2631cad5b481777a68e8725f34e89f50fc6ac714e379b40  go1.27.3.linux-amd64.tar.gz' | sha256sum --check -
    ```

    - 1.27.3は手順の例。別の版ではファイル名・URL・SHA-256をすべて同じ版に変更。
    - 成功の目印：`go1.27.3.linux-amd64.tar.gz: OK`。検証に失敗した場合は次へ進まない。

2. 検証成功後、公式手順どおり既存の `/usr/local/go` を削除し、新しいツリーを展開。

    ```bash
    sudo rm -rf /usr/local/go
    sudo tar -C /usr/local -xzf go1.27.3.linux-amd64.tar.gz
    ```

    - `/usr/local/go` の既存ツールチェーンを置き換えます。既存ツリーへの上書き展開は避けてください。

3. 下の行を `~/.profile` に追加し、現在のシェルでも実行。

    ```bash
    export PATH="/usr/local/go/bin:$PATH"
    ```

4. 使用するGoを確認。

    ```bash
    command -v go
    go version
    ```

    ```text
    /usr/local/go/bin/go
    go version go1.27.3 linux/amd64
    ```

    - 出力の版は導入した版に対応。別のGoが表示される場合はPATHの順序を確認。

## 3. uvとPythonを準備する

**uvが未導入の場合だけ、[公式の版指定インストーラー](https://docs.astral.sh/uv/getting-started/installation/#standalone-installer)を実行。**

```bash
curl -LsSf https://astral.sh/uv/0.13.5/install.sh | sh
export PATH="$HOME/.local/bin:$PATH"
uv python install 3.12
uv --version
uv python find --no-project --no-config --managed-python --system 3.12
```

- uv 0.13.5はCIと同じ版の例。導入済みならPythonの準備・確認だけ実施。
- 最後のコマンドでuv管理Python 3.12の絶対パスが表示されれば準備完了。
- OSのPythonは変更しません。

## 4. ビルド・検証へ進む

- [ソースからビルドして配置](../setup.md#ソースからビルドする場合)
- [コードを確認する](../testing.md#コードを確認する)
- [ドキュメントを追加・修正する](documentation.md)
- [リリースの作成](releases.md)
