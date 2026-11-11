# Sprint 7 — Operación: backups, logs, métricas

## Objetivo

Operar Alpha: backups, retención de logs, métricas de nodo, alertas mínimas.

## Estado

**Implementado el 2026-11-10.** Siguiente: S8 (hardening).

## Entregables

- Migración `000009`: métricas en `nodes`, tablas `backups` + `alert_events`.
- Heartbeat persiste CPU/memoria; UI Nodes muestra barras de métricas.
- Retención de `workload_logs` / `build_logs` (worker ops).
- Backups `pg_dump` vía UI Ops + `scripts/backup-postgres.sh` / `restore-postgres.sh`.
- Alertas `node.down` / `deploy.failed` (+ webhook opcional `NORIX_ALERT_WEBHOOK_URL`).
- Runbook: `docs/runbooks/S7_OPERATIONS.md`.
- UX: Domains rediseñado, ActivityBanner en proyectos, badges busy, links HTTPS `:8443`.

## DoD

| Criterio | Estado |
|---|---|
| Backup + restore documentado/verificable | Cumple |
| Métricas nodo en dashboard | Cumple |
| Alerta al caer agente remoto | Cumple (45s stale) |
| Retención logs | Cumple |

## Nota dominio HTTPS

Abrir siempre `https://hostname:8443` (edge local). Certificado internal: aceptar excepción del navegador en `*.localhost`.
