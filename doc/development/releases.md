# リリースの作成

## 配布物

| ファイル | 内容 |
|---|---|
| `oaiso-linux-amd64.tar.gz` | 静的CLI・起動検証／ログ用プログラム・版とSHA-256・ライセンス／notice |
| `install.sh` | そのリリースの版を埋め込んだ単独インストーラー |
| `SHA256SUMS` | 配布アーカイブとインストーラーのSHA-256 |

- 対象ホストはUbuntu 26.04 x86_64です。
- `go.mod` のGo版、uv 0.13.5、uv管理Python 3.12で生成します。
- 作業イメージ・Kata等の固定配布物・認証情報は、このアーカイブには含みません。

## ローカルで配布物を確認する

```bash
release_tag=v0.0.3
uv python install 3.12
make node-bundle VERSION="$release_tag"
uv run --no-project --no-config --managed-python --python 3.12 python scripts/package-release.py \
  --bundle dist/node-bundle --output dist/release --version "$release_tag"
cd dist/release
sha256sum --check SHA256SUMS
bash -n install.sh
```

- `release_tag` は次に作成する版です。
- バイナリの版・SHA-256・Linux amd64 ELF・静的リンクを検査します。
- Goと実際の依存モジュールのライセンス／noticeを同梱します。
- 通常CIでも `v0.0.0-ci` の配布物を生成して整合性を確認します。

## 公開する

1. `doc/release-notes.md` を更新し、先頭を `# Oaiso vX.Y.Z` にします。
2. 変更をmainへcommit・pushし、CIの成功を確認します。
3. 同じcommitにリリースタグを作成・pushします。

```bash
git tag -a "$release_tag" -m "Oaiso $release_tag"
git push origin "$release_tag"
```

- `Release` workflowがGo/Pythonテスト、ビルド、アーカイブ生成、SHA-256確認を実行します。
- 下書きリリースへ全ファイルを添付してから公開します。
- 安定版は最新版、`-rc`等を含む版はプレリリースとして公開します。
- 公開済みリリースは上書きしません。修正を配布する場合は新しい版を作ります。
- 未公開の下書きを再試行する場合は、Actionsの `Release` を既存タグとともに手動実行できます。`publish=false` なら下書きのまま保持します。

## 公開後に確認する

- リリースの3ファイルが取得可能で、SHA-256が一致すること。
- 公開した `install.sh` を通常ユーザーでパイプ実行し、選択版の配置計画を表示できること。
- 完全な配置・更新・復旧は、[使い捨てKataノードの試験](../testing.md#新規導入更新復旧を検証する)で確認します。
