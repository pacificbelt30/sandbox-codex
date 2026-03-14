# Network Specification (dock-net)

> [日本語](../network.md) | **English**

codex-dock uses a dedicated Docker bridge network **dock-net** to isolate worker containers.

---

## Current Network / Firewall Specification

### dock-net base configuration

| Item | Value |
|---|---|
| Network name | `dock-net` |
| Driver | `bridge` |
| Bridge name | `dock-net0` |
| Subnet | `10.200.0.0/24` |
| Gateway | `10.200.0.1` |
| ICC | `false` (inter-container traffic disabled) |
| IP Masquerade | `true` by default (`false` with `--no-internet`) |

### Linux firewall rules

On Linux, codex-dock manages `iptables` rules by linking `DOCKER-USER` to a managed chain named `CODEX-DOCK`.

Rule application order:

1. (When `dock-net-proxy0` exists) insert NIC-level allow rules into `DOCKER-USER`
   - `-i dock-net0 -o dock-net-proxy0 -p tcp --dport <proxy-port> -j ACCEPT`
   - `-i dock-net-proxy0 -o dock-net0 -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT`
2. Insert `-i dock-net0 -j CODEX-DOCK` into `DOCKER-USER`
3. Flush `CODEX-DOCK`
4. Add allow rules for:
   - `IP:PORT` parsed from `--proxy-container-url`
   - `dock-net` subnet to the same proxy port
5. Drop private/link-local destinations (`10/8`, `172.16/12`, `192.168/16`, `169.254/16`, `127/8`)
6. Add final `RETURN`

> `codex-dock run` attempts firewall setup automatically. If root privileges are missing or `iptables` is unavailable, it continues with a warning.

---

## Command Behavior

### `codex-dock firewall create`

```bash
codex-dock firewall create [--no-internet] [--proxy-container-url URL]
```

- Applies rules on Linux when run as root and `iptables` is available.
- If `dock-net` is missing, codex-dock prints a warning and prompts whether to create it (if declined, firewall setup stops).
- If `dock-net-proxy` is missing, codex-dock prints a warning and prompts whether to create it.
- Default `--proxy-container-url` is `http://codex-auth-proxy:18080`.

### `codex-dock firewall status`

```bash
codex-dock firewall status
```

Shows:

- Linux support
- Root execution status
- `iptables` detection
- `CODEX-DOCK` chain existence
- `DOCKER-USER -> CODEX-DOCK` jump rule existence
- `DOCKER-USER` default policy
- Final jump verdict in `CODEX-DOCK`

### `codex-dock firewall rm`

```bash
codex-dock firewall rm
```

Removes the `DOCKER-USER -> CODEX-DOCK` jump rule and deletes the `CODEX-DOCK` chain.

---

## Notes

- macOS / Windows (Docker Desktop) do not have equivalent automatic Linux `iptables` control.
- `network create` provisions the Docker network only; it does not install firewall rules.
