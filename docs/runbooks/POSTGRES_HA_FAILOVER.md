# Postgres HA (Patroni) — runbook and RPO (#52)

Local / lab HA for the Norix control-plane database: **etcd + 2 Patroni/Postgres nodes + HAProxy**.

> Not a managed cloud offering. Use this to validate failover before production (or prefer a managed Postgres with HA SLA).

## Topology

| Component | Role |
|---|---|
| etcd | DCS for Patroni leader election |
| `norix-patroni-1` / `-2` | Postgres 16 + Patroni |
| HAProxy `:5435` | TCP VIP → current **primary** only (`/primary` check on `:8008`) |

Compose: `deploy/norix/docker-compose.patroni.yml`  
Details / ports: [`deploy/postgresql/patroni/README.md`](../../deploy/postgresql/patroni/README.md)

## Start

```bash
cd deploy/norix
docker compose -f docker-compose.patroni.yml up -d
# Wait until HAProxy accepts SQL:
PGPASSWORD=norix-patroni psql -h 127.0.0.1 -p 5435 -U postgres -c 'SELECT 1'
```

Point the control plane at the VIP:

```bash
export DATABASE_URL='postgres://postgres:norix-patroni@127.0.0.1:5435/norix?sslmode=disable'
export MIGRATION_DATABASE_URL="$DATABASE_URL"
```

Create the app DB once (`CREATE DATABASE norix;`) if it does not exist, then migrate and start the CP as usual.

## RPO / RTO (documented)

| Metric | Lab expectation | Notes |
|---|---|---|
| **RPO** | ≤ ~1 MiB of WAL lag | Patroni `maximum_lag_on_failover: 1048576`. Replication is **asynchronous** by default — a crash can lose the last un-replicated transactions. |
| **RTO** | typically &lt; 60–90s | Leader election + HAProxy health (3s × fall 3) + CP pool reconnect. |

For stricter RPO enable synchronous replication in Patroni DCS (trade latency / availability). See [Patroni replication](https://patroni.readthedocs.io/en/latest/replication_modes.html).

## Manual failover drill

1. Identify primary: `curl -s http://127.0.0.1:8008/patroni | jq .role` (and `:8009`).
2. Insert a marker row through HAProxy (`5435`).
3. Hard-kill the primary (`docker kill` / SIGKILL) — softer `docker stop` is also fine for drills.
4. Wait until the survivor reports `master`/`primary` and `psql` on `5435` works.
   Ensure data dirs are mode `0700` (entrypoint + smoke chmod); Postgres refuses to promote otherwise.
5. `SELECT` the marker — must still be present (within RPO).
6. Confirm control plane `/healthz` and a DB-backed RPC (e.g. `GetBootstrapStatus`) succeed after reconnect.

## Automated smoke

```bash
./test/patroni/failover.sh
# NORIX_PATRONI_KEEP=1 leaves the stack up
# SKIP_PATRONI_FAILOVER=1 skips the CI job
```

The script builds the control plane, runs migrations against HAProxy, stops the primary, asserts the marker and CP reconnect.

## Production notes

- Prefer managed Postgres HA (RDS Multi-AZ, Cloud SQL HA, etc.) unless you operate Patroni/etcd yourself.
- Always connect apps (including Norix) to a **primary VIP** or proxy that follows the leader — never hard-code one replica.
- Backups / PITR remain required; HA is not a substitute for backups ([S7_OPERATIONS](./S7_OPERATIONS.md)).
