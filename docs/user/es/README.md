# Guía de usuario de Norix

Documentación oficial para quien **usa la plataforma** (dashboard), no para instalar el control plane.

| | |
|---|---|
| **Instalar Norix** (operadores) | [`README.es.md`](../../../README.es.md) · [`BARE_INSTALL`](../../runbooks/BARE_INSTALL.md) |
| **Usar el dashboard** | Empieza en [Primeros pasos](./getting-started.md) |
| **English (canónico)** | [`../README.md`](../README.md) |
| **Actualizar / desinstalar** | [`UPDATE_AND_UNINSTALL.md`](../../UPDATE_AND_UNINSTALL.md) |

## Contenido

1. [Primeros pasos](./getting-started.md)
2. [Mapa del dashboard](./dashboard.md)
3. [Proyectos y workloads](./projects-workloads.md)
4. [Desplegar](./deploying.md)
4b. [Historial de releases y rollback](./releases-rollback.md) — snapshots inmutables, restore en un clic
4c. [Updates rolling](./rolling-updates.md) — start-before-stop, maxSurge / maxUnavailable
4d. [Probes](./probes.md) — liveness / readiness HTTP·TCP·exec
4e. [Volúmenes persistentes](./volumes.md) — montajes Docker, nodo sticky, backup/delete
5. [Nodos y agents](./nodes-agents.md)
5b. [Socket Docker](./docker-socket.md) — quién monta `docker.sock`, proxy/rootless
5c. [Autenticación del agent](./agent-auth.md) — rotar/revocar token, mTLS opcional
6. [Dominios y TLS](./domains.md)
7. [Bases de datos](./databases.md)
8. [Red y edge](./network.md)
9. [Ops y backups](./ops.md)
10. [Acceso y equipos](./access.md)
11. [Solución de problemas](./troubleshooting.md)

## Qué es Norix

Plano de control PaaS **autoalojado**. Desde la UI creas proyectos, despliegas desde imagen o Git, unes workers con agents, dominios con TLS, bases de datos y operaciones (backups).

Estado beta: [`ESTADO_Y_PLAN_V1.md`](../../ESTADO_Y_PLAN_V1.md). Versión actual del CP: badge bajo el logo **N** o `GET /status`.
