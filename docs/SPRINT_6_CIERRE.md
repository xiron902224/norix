# Sprint 6 — Edge, dominio, TLS (Alpha 0.1)

## Objetivo

Hostname → workload con HTTPS vía Caddy; ACME local con Pebble; rollback de config.

## Estado

**Implementado el 2026-11-10.** Siguiente: S7 (ops/backup/observabilidad).

## Entregables

- Proto: `AssignDomain` / `ListDomains` / `UnassignDomain`; agent `DesiredEdgePlan` + `ReportEdge`.
- Migración `000008`: `domain_routes`, `edge_snapshots`.
- Caddy + Pebble en `deploy/norix` (HTTP `:8088`, HTTPS `:8443`, admin `:2019`).
- Localruntime + agente: cargan config Caddy (`POST /load`), health-check upstream, rollback al config previo.
- UI **Domains**: asignar hostname, TLS auto/internal/acme, quitar ruta.
- `scripts/install.sh`: `up|upgrade|uninstall|checksums|status`.
- Upstream host configurable (`NORIX_EDGE_UPSTREAM_HOST=host.docker.internal` para Caddy en Docker).

## DoD

| Criterio | Estado |
|---|---|
| Dominio → workload vía edge | Cumple |
| TLS internal en `*.localhost` | Cumple |
| ACME vía Pebble (configurado) | Cumple (VA always valid en compose) |
| Health check + rollback | Cumple |
| install upgrade/uninstall + checksums | Cumple |

## Demo local

```bash
docker compose --env-file .env -f deploy/norix/docker-compose.yml up -d postgres pebble caddy
cd control-plane && set -a && . ./.env && set +a && go run ./cmd/server
# UI embebida en :8080 o Vite en :5173
# Deploy workload → Domains → app.localhost → https://app.localhost:8443
```

## Notas

- Puertos 80/443 del host se mapean a 8088/8443 para no requerir root.
- Multi-nodo edge scheduling queda para post-S6; rutas se aplican en el nodo asignado al workload.
