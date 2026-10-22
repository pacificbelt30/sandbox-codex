# `oaiso network` — Network Management

> [日本語](../../commands/network-cmd.md) | **English**
>
> [← Command Reference](../commands.md)

Manages the egress network (`dock-net-proxy`, used for the proxy's internet reachability). Per-worker `Internal` networks are created and removed automatically with each worker, so no manual management is needed.

> The old `oaiso firewall` (iptables) command has been removed; isolation is enforced by the root node service with Linux bridges, namespaces and nftables.

---

## `network create`

```bash
oaiso network create
```

> Also auto-created by `oaiso proxy run` if missing.

---

## `network rm`

```bash
oaiso network rm
```

> Stop/remove the proxy container first if it is still attached.

---

## `network status`

```bash
oaiso network status
```

```
Egress network:  dock-net-proxy
  ID:            a1b2c3d4e5f6
  Driver:        bridge
  Internal:      false
  Subnet:        10.253.0.0/24
Worker networks: 2 (Internal, one per worker)
  - dock-net-w-codex-brave-otter
  - dock-net-w-codex-calm-finch
```

---

## Related Documentation

- [Network Specification](../network.md)
