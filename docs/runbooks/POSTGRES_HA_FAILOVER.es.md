# Postgres HA (Patroni) — runbook y RPO (#52)

HA de laboratorio para la base del control plane: **etcd + 2 nodos Patroni/Postgres + HAProxy**.

> No es un servicio gestionado. Sirve para validar failover antes de producción (o usa Postgres gestionado con SLA de HA).

## Topología

| Componente | Rol |
|---|---|
| etcd | DCS / elección de líder |
| `norix-patroni-1` / `-2` | Postgres 16 + Patroni |
| HAProxy `:5435` | VIP TCP → **primario** actual |

Compose: `deploy/norix/docker-compose.patroni.yml`  
Detalle: [`deploy/postgresql/patroni/README.md`](../../deploy/postgresql/patroni/README.md)

## Arranque

```bash
cd deploy/norix
docker compose -f docker-compose.patroni.yml up -d
PGPASSWORD=norix-patroni psql -h 127.0.0.1 -p 5435 -U postgres -c 'SELECT 1'
```

Apunta el CP al VIP (`DATABASE_URL` / `MIGRATION_DATABASE_URL` en el puerto `5435`).

## RPO / RTO

| Métrica | Expectativa lab | Notas |
|---|---|---|
| **RPO** | ≤ ~1 MiB de lag WAL | `maximum_lag_on_failover: 1048576`. Replicación **asíncrona** por defecto. |
| **RTO** | &lt; 60–90 s | Elección + health HAProxy + reconnect del pool del CP. |

Guía canónica EN: [POSTGRES_HA_FAILOVER.md](./POSTGRES_HA_FAILOVER.md).

## Drill manual (resumen)

1. Identifica primario (`:8008` / `:8009`).
2. Inserta un marker por HAProxy (`5435`).
3. `docker kill` del primario (crash duro).
4. Espera promoción; data dir debe ser modo `0700`.
5. Comprueba marker + `/healthz` del CP.

## Smoke automatizado

```bash
./test/patroni/failover.sh
# NORIX_PATRONI_KEEP=1 deja el stack arriba
```
