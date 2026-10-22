# Network

[日本語](../network.md) | **English**

The root node service manages Linux bridges, namespaces, veth links and dedicated nftables tables.
It does not depend on Docker network APIs or a CNI daemon.

## Connections

| Network or proxy | Purpose |
|---|---|
| `dock-net-proxy` | Primary proxy network, host gateway and outbound NAT |
| `dock-net-w-<name>` | Private worker L2 network, without a host IPv4 address, default route or NAT |
| `codex-auth-proxy` | Fixed AI reverse routes and token API on connected worker networks |
| `codex-http-proxy` | General HTTP/CONNECT forwarding, attached only when allowed |

Each worker has one private network. Kata workers receive proxy URLs pinned to
the verified front IPs on that network, without relying on the guest hosts file.
The worker uses loopback DNS; external resolution and dialing happen in the proxies.
Proxy namespaces do not forward IP packets between interfaces.

Dedicated nftables rules block worker-to-host and worker-to-other-interface traffic.
Host IPv4 forwarding and existing firewall rules must allow the proxy egress path.
The product does not flush other services' tables.

## Ports

| Listener | Exposure |
|---|---|
| Auth data `18080` | Worker networks only |
| Auth admin `18081` | Primary egress IP, relayed to host `127.0.0.1` |
| General forward `18082` | Selected worker networks only |

Workers cannot reach the admin listener or management sockets.
Only the auth proxy receives real credentials.

## Outbound policy

General egress supports domain, IP/CIDR and port allow/deny rules.
Denials win. All nonempty allowlists must match.
Private, loopback, link-local and metadata addresses remain blocked.
DNS answers and redirects are checked, and the dialer uses a verified address.

```bash
oaiso proxy run \
  --forward-allow-domain github.com \
  --forward-allow-domain registry.npmjs.org \
  --forward-deny-domain blocked.github.com
```

These rules control general egress, not fixed AI authentication routes.
`run --no-internet` disconnects the general egress proxy and keeps AI routes available.
See [policy rules](../proxy-policy.md) (Japanese).

## Lifecycle

Stop keeps networks for restart. Remove detaches the selected proxies and reclaims owned resources.
Network IDs, addresses, namespace identity and firewall state are recorded privately and verified before reuse.
Subnets are selected from `10.253.0.0/16`; existing addresses and routes are checked for conflicts.

```bash
oaiso network create
oaiso network status
oaiso network rm
```

See [host setup](../kata.md) and [troubleshooting](../troubleshooting.md) (Japanese).
