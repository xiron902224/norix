# Sprint 9 — Observabilidad y dashboard en vivo

## Objetivo (plan S0–S22)

Métricas de nodo/carga, ingest y búsqueda de logs, exportación Prometheus, alertas básicas, vista en vivo.

## Estado

**Cerrado 2026-11-13.**

## Entregables

- Tablas `metric_samples`, `log_segments`, `alert_rules`, `alert_events` (migración `000013`).
- Heartbeat / local runtime → `SampleNodeHostMetrics` → samples.
- Ingest HTTP `POST /api/ops/logs/ingest` + `QueryLogs` con filtro `query` (substring).
- Retención: workload/build logs + `RetainLogSegments` / `RetainMetricSamples` en worker ops.
- Export Prometheus `GET /metrics?org_id=…`.
- UI Observability: nodos en vivo, búsqueda de logs, alert events.
- Alertas ops `node.down` / `deploy.failed` (+ webhook) ya en runbook S7 ops.

## DoD

| Criterio | Estado |
|---|---|
| Métricas nodo + samples | Cumple |
| Búsqueda de logs | Cumple (`query` + UI) |
| Export Prometheus | Cumple |
| Alertas básicas | Cumple |
| Retención segments/samples | Cumple |
| Tests | `prometheus_test`, `observability_filter_test` |

## Cómo verificar

```bash
cd control-plane && go test ./internal/api/ ./internal/db/ -count=1 -run 'SanitizeProm|FilterLog'
# UI: Dashboard → Observability → Search logs
curl -s "http://127.0.0.1:8080/metrics?org_id=<org-uuid>" | head
```
