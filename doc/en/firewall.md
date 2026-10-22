# Removed firewall feature

The old `oaiso firewall` and `--allow-host` / `--block-host` / `--no-firewall` / `--sudo` flags have been removed.

The root node service now enforces isolation with dedicated Linux bridges, namespaces and nftables tables.
Host setup requires administrator privileges. Normal CLI use goes through the operator group.

Use [network isolation](network.md) for direct access restrictions and [proxy policies](../proxy-policy.md) (Japanese) for general egress allow/deny rules.
`run --no-internet` disconnects general egress while keeping AI API routes.
See [migration](../migration.md) (Japanese).
