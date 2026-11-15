# Patroni local HA stack

Minimal 2-node Postgres + etcd + Patroni + HAProxy for local HA experiments.

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

Failover smoke: stop the primary container (`docker stop norix-patroni-1` or `-2`) and re-check `/patroni` on the survivor + HAProxy `5435`.

## Notes

- Uses `postgres:16-alpine` with Patroni installed at container start (`apk add patroni`).
- DCS is etcd v3. First boot elects a leader and initializes the cluster.
- Not for production; intended for local Norix HA validation alongside control-plane leases.
