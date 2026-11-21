# Chaos tests

Scripted failure scenarios for a lab or nightly CI. They document **expected behavior** and automate the closest assertions on one Docker host (same limitation as [multi-node smoke](../../test/multinode/README.md): agents share `docker.sock`).

| Scenario | Script | Trigger |
|---|---|---|
| Agent crash | `test/chaos/agent-crash.sh` | `docker kill` agent container |
| Control-plane restart | `test/chaos/control-plane-restart.sh` | Compose restart CP |
| Database restart | `test/chaos/db-restart.sh` | Compose restart Postgres |
| Network partition | `test/chaos/network-partition.sh` | `docker network disconnect` |

Run all (shared stack):

```bash
./test/chaos/run-all.sh
```

Nightly: `.github/workflows/chaos-nightly.yml` (`schedule` + `workflow_dispatch`).

## Expected behavior

### Agent crash

1. Heartbeats stop.
2. Within ~45–90s (15s leader tick + 30s stale window) the node status becomes **`offline`**.
3. Orphan replicas are eligible for reschedule onto other online, non-cordoned workers (`RescheduleAllOrphans`).
4. Remaining agents keep heartbeating.

**Asserted in CI:** killed agent → `offline`; ≥1 other node still `online`.

### Control-plane restart

1. `/healthz` fails briefly while the process is down.
2. After restart, `/healthz` returns 200; migrations are not re-applied destructively.
3. DB-backed sessions remain valid; agents reconnect and resume heartbeats.
4. Desired state is re-loaded from Postgres — no need to re-enroll nodes that still have node tokens.

**Asserted in CI:** healthz recovers; previous online agent count returns within ~3 minutes.

### Database restart (Compose Postgres)

1. During the outage, API calls that need the DB fail; `/healthz` may fail depending on pool checks.
2. After Postgres is healthy again, the control plane should serve again. If the process wedged on broken connections, **restart the control-plane container** (documented recovery).
3. Agents keep retrying; once CP+DB are up they show `online` again.

**Asserted in CI:** postgres restart → (optional CP restart) → healthz + ≥1 online agent.

> Full **Patroni failover** is covered by [`docs/runbooks/POSTGRES_HA_FAILOVER.md`](../runbooks/POSTGRES_HA_FAILOVER.md) and `./test/patroni/failover.sh` (#52), not by these chaos scripts.

### Network partition

1. Disconnect the agent from the compose network (simpler and safer than host `iptables` in CI).
2. Heartbeats cannot reach the CP → node becomes **`offline`** after the stale window.
3. Reconnect the network; if the agent process is still running it resumes heartbeats → **`online`**.

**Asserted in CI:** offline during disconnect, online after reconnect.

## Limitations

- Not a multi-host mesh or WireGuard partition test.
- Not a substitute for load/`k6` or Playwright UI E2E.
- Shared Docker socket means container scheduling across “nodes” is logical, not physically isolated.

## Related

- Multi-node smoke: `test/multinode/smoke.sh` (#37)
- Ops: [ops.md](./ops.md) · Nodes: [nodes-agents.md](./nodes-agents.md)
- Español: [es/chaos.md](./es/chaos.md)
