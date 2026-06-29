# `codex-dock firewall` — 廃止

> **日本語** | [English](../../en/commands/firewall.md)
>
> [← コマンドリファレンス一覧](../commands.md)

このコマンドは削除されました。ネットワーク隔離は Docker のネイティブ機能（per-worker `Internal` ネットワーク + プロキシルータ）で実現します。`iptables`/`sudo` は不要です。

- 概要: [ネットワーク仕様](../network.md)
- egress のドメイン制限: `codex-dock proxy run --forward-allow-domain <domain>`
