# Allow general network traffic

[日本語](../../guides/networking.md) | **English**

| Traffic | Default |
|---|---|
| Fixed Codex/Claude AI routes | Authorized by live placeholders, provider and model rules |
| Git, npm, pip and arbitrary websites | General egress; disconnected without a domain or CIDR grant |
| Direct VM traffic, host and other workers | Blocked |

## 1. Configure host destination grants

- File: `~/.config/codex-dock/config.toml`.
- This example grants GitHub and npm. Add actual distribution and redirect destinations as needed.
- Merge into the existing `[proxy]` table; retain `admin_secret_file` and other keys.

```toml
[proxy]
forward_default_deny = true
forward_allow_domains = ["github.com", "registry.npmjs.org"]
forward_allow_ports = [80, 443]
```

| Setting | Meaning |
|---|---|
| `github.com` | Domain and subdomains |
| `=api.github.com` | Exact hostname only |
| Ports alone | Do not grant destinations |
| Domain and CIDR allowlists together | Both must match; denials always win |
| Private, loopback and metadata IPs | Cannot be unblocked with a CIDR grant |

- GitHub asset downloads may use other domains not granted by this example.
- SSH, JVM and custom CA behavior: [compatibility settings](../sandbox-features.md).

## 2. Check before launch

```bash
oaiso policy check --json
oaiso policy explain registry.npmjs.org:443 --json
```

**Output of the second command with the grants above**

```json
{
  "destination": {"Host": "registry.npmjs.org", "Port": 443},
  "allowed": true,
  "code": "address_not_checked",
  "stage": "destination",
  "addresses_checked": 0,
  "complete": false
}
```

- Formatted for readability; the CLI emits one line.
- `allowed: true`, `complete: false`: hostname and port passed; DNS IP checks have not run. This does not prove live connectivity.
- A valid `policy check` returns exit code `0` and policy JSON with `source: configuration for NEW workers; not a live update`.

**Also check a destination without a grant**

```bash
oaiso policy explain example.com:443 --json
```

```json
{
  "destination": {"Host": "example.com", "Port": 443},
  "allowed": false,
  "code": "domain_not_allowed",
  "stage": "destination",
  "addresses_checked": 0,
  "complete": true
}
```

- Denial is expected here. `explain` itself succeeds; inspect `allowed` and `code` rather than the command's exit status for the policy decision.
- Without destination grants: `code: general_egress_disabled`. Fixed AI routes are separate.

- These inspect host settings for new workers; they do not change live traffic.
- `explain` opens no connection; this example checks hostname and port only.
- Add `--resolve` to check DNS result IPs; this still does not test live service connectivity.
- Interactive editing: `oaiso proxy configure`, also available when the node is stopped.

## 3. Apply to new workers

| Current state | Action |
|---|---|
| First setup | Continue to `init` in the [quick start](../getting-started.md#3-prepare-images-and-start-the-proxies) |
| Existing proxies/workers | Save results, stop/remove workers, stop/remove affected proxies, run `init` with the new settings, create new workers |

- Saving settings does not update existing proxies, CONNECT tunnels or workers.
- Procedure and names: [proxy recreation](../../operations.md#cliプロキシを更新する) (Japanese).
- Per-worker `--egress-allow-domain` and presets narrow host grants; they cannot add destinations denied by the host.

## 4. Inspect or stop a worker

```bash
oaiso policy show --worker WORKER --json
oaiso policy events --worker WORKER --json
```

- Replace `WORKER` with a name from `oaiso ps -a`.
- Denial history is shared by workers on the same egress proxy; it is not an independent per-run audit log.
- Immediate cutoff: `oaiso policy quarantine --worker WORKER`. **Stops communication and all jobs, and forbids restarting that worker.**
- Retention, artifacts and failure handling: [quarantine contract](../sandbox-features.md#stop-communication-and-quarantine).
- Detailed rules: [network policy](../../proxy-policy.md) (Japanese).
