# Patroni local HA stack

Minimal 2-node Postgres + etcd + Patroni + HAProxy for local HA experiments.

**Operator runbook (RPO/RTO, CP reconnect, automated smoke):**  
[`docs/runbooks/POSTGRES_HA_FAILOVER.md`](../../../docs/runbooks/POSTGRES_HA_FAILOVER.md) · [ES](../../../docs/runbooks/POSTGRES_HA_FAILOVER.es.md)

## Start

```bash
cd deploy/norix
docker compose -f docker-compose.patroni.yml up -d
```

Optional passwords:

```bash
export PATRONI_SUPERUSER_PASSWORD=norix-patroni
export PATRONI_REPLICATION_PASSWORD=norix-repl
```

## Endpoints

| Port | Role |
|------|------|
| `127.0.0.1:5435` | HAProxy → current primary |
| `127.0.0.1:5433` | patroni1 direct |
| `127.0.0.1:5434` | patroni2 direct |
| `127.0.0.1:8008` | patroni1 REST |
| `127.0.0.1:8009` | patroni2 REST |
| `127.0.0.1:7000` | HAProxy stats |

## Checks

```bash
curl -s http://127.0.0.1:8008/patroni | head
curl -s http://127.0.0.1:8009/patroni | head
PGPASSWORD=norix-patroni psql -h 127.0.0.1 -p 5435 -U postgres -c 'SELECT pg_is_in_recovery();'
```

## Automated failover (#52)

```bash
./test/patroni/failover.sh
```

Stops the current primary, waits for promotion through HAProxy, asserts a marker row still exists (within documented RPO), and checks the control plane `/healthz` + `GetBootstrapStatus` after pool reconnect.

## Notes

- Builds `norix/patroni-pg:local` from `deploy/postgresql/patroni/Dockerfile` (Postgres 16 Alpine + Patroni via pip) + etcd v3 + HAProxy.
- First boot elects a leader and initializes the cluster.
- Replication is asynchronous; RPO is documented in the runbook (lab default ≈ last unreplicated WAL, capped by Patroni lag settings).
- Not for production; intended for local Norix HA validation alongside control-plane leases.
