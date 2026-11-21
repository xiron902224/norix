# Pruebas de caos

Escenarios de fallo para lab o CI nightly. Documentan el **comportamiento esperado** y automatizan las aserciones alcanzables en un solo host Docker (misma limitación que el [smoke multi-nodo](../../../test/multinode/README.md): los agents comparten `docker.sock`).

| Escenario | Script | Disparo |
|---|---|---|
| Crash del agent | `test/chaos/agent-crash.sh` | `docker kill` del contenedor |
| Restart del control plane | `test/chaos/control-plane-restart.sh` | Compose restart CP |
| Restart de la DB | `test/chaos/db-restart.sh` | Compose restart Postgres |
| Partición de red | `test/chaos/network-partition.sh` | `docker network disconnect` |

Todo el suite:

```bash
./test/chaos/run-all.sh
```

Nightly: `.github/workflows/chaos-nightly.yml`.

## Comportamiento esperado

### Crash del agent

1. Dejan de llegar heartbeats.
2. En ~45–90s el nodo pasa a **`offline`**.
3. Las réplicas huérfanas pueden reprogramarse a workers online no cordonados.
4. El resto de agents sigue en línea.

### Restart del control plane

1. `/healthz` falla un momento.
2. Tras el restart vuelve 200; el estado desired sale de Postgres.
3. Las sesiones en DB siguen; no hace falta re-enroll si el node token sigue válido.

### Restart de Postgres

1. Durante la caída fallan las llamadas que necesitan DB.
2. Cuando Postgres está sano, el CP debería responder; si el pool quedó roto, **reinicia el contenedor del control plane**.
3. Los agents reintentan hasta volver `online`.

> Failover Patroni: ver [`POSTGRES_HA_FAILOVER.es.md`](../../runbooks/POSTGRES_HA_FAILOVER.es.md) y `./test/patroni/failover.sh` (#52).

### Partición de red

1. Se desconecta el agent de la red Compose.
2. Sin heartbeats → **`offline`**.
3. Al reconectar, si el proceso sigue vivo → **`online`**.

## Limitaciones

- No es una prueba multi-host ni de mesh WireGuard.
- Un solo `docker.sock` compartido.

Canónico EN: [../chaos.md](../chaos.md).
