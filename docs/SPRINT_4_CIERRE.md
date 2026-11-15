# Sprint 4 — Reconciliación + runtime contenedores

## Objetivo

Desired state → contenedor real vía agente; Deploy usable desde el dashboard.

## Estado

**Implementado el 2026-11-10.** Siguiente: S5 (git/build/registry).

## Entregables

- Desired/observed versionado: `desired_generation`, `observed_generation`, `desired_action`, puertos, `assigned_node_id`.
- Tablas `deployments` + `workload_logs` con RLS.
- `Deploy` asigna nodo online, puerto host (18080+) y marca desired `run`.
- Heartbeat devuelve `desired_workloads`; `ReportObserved` aplica estado + logs.
- Agente: reconciler idempotente con Docker o nerdctl (pull/run/rm/logs).
- UI: Deploy, endpoint, generaciones, drawer de logs; overview actualizado.
- `GetWorkloadLogs` para el dashboard.

## DoD

| Criterio | Estado |
|---|---|
| Deploy imagen pública (p.ej. nginx:alpine) | Cumple (requiere agente + docker/nerdctl) |
| Puerto host mapeado + endpoint en UI | Cumple |
| Agente caído y vuelta → reconverge | Cumple (desired permanece; heartbeat re-aplica) |
| Logs mínimos visibles | Cumple |

## Notas

- Runtime preferido: `docker`; fallback `nerdctl` (containerd).
- Sin motor de contenedores el agente sigue en heartbeat-only.
- Git→build y registry quedan para S5.
