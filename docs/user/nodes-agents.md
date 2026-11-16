# Nodes & agents

Agents are processes on your machines that enroll with the control plane, heartbeat, and reconcile containers.

## Join a node

1. **Regions & nodes** → **Create join token** (optional suggested name)
2. On the worker:

```bash
curl -fsSL https://raw.githubusercontent.com/xiron902224/norix/v1.0.0-beta.4/scripts/install-agent.sh \
  | bash -s -- --version v1.0.0-beta.4 \
      --control-plane 'https://YOUR_CP' \
      --token 'JOIN_TOKEN'
```

Labs without systemd: add `--supervise none`.

3. Confirm **online**, hostname, **Agent version**, **Docker = yes**

## Schedule controls

- **Cordon** — no new workloads
- **Drain** — evacuate then cordon behavior for migration
- **Role / zone** — scheduling hints (`worker` / `edge`, zone labels)

## Update agents

Keep agent versions aligned with the control plane.

| Path | How |
|---|---|
| **UI** | Select node → **Update agent** (pins `desired_agent_version`; agent self-updates on heartbeat) |
| **CLI on the node** | `norix-agent update --version v1.0.0-beta.4` or `install-agent.sh --update --version …` |
| **Check** | `norix-agent check` (no join token required) |

Full detail: [`UPDATE_AND_UNINSTALL.md`](../UPDATE_AND_UNINSTALL.md).

## Uninstall agent

```bash
norix-agent uninstall [--purge]
# or scripts/uninstall-agent.sh
```
